package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func testPolicySpec(t *testing.T, name string) policySpec {
	t.Helper()
	for _, spec := range policies {
		if spec.name == name {
			return spec
		}
	}
	t.Fatalf("Unknown policy %q", name)
	return policySpec{}
}

func testPolicy(t *testing.T, name string, handler http.HandlerFunc) *policyResource {
	t.Helper()
	return testPolicyWithSpec(t, testPolicySpec(t, name), handler)
}

func testPolicyWithSpec(t *testing.T, spec policySpec, handler http.HandlerFunc) *policyResource {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get("Authorization") != "Bearer test-token" || req.Header.Get("X-GitHub-Api-Version") != "2026-03-10" {
			t.Error("Missing expected GitHub API request headers")
		}
		handler(w, req)
	}))
	t.Cleanup(server.Close)
	client := newGitHubClient("test-token")
	client.baseURL = server.URL
	client.interval = 0
	return &policyResource{spec: spec, client: client}
}

func TestPullRequestPolicyWritesOnlyItsFields(t *testing.T) {
	var changed atomic.Bool
	policy := testPolicy(t, "repository_pull_requests", func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/repos/owner/repo" {
			t.Errorf("Unexpected path %s", req.URL.Path)
		}
		switch req.Method {
		case http.MethodGet:
			policy := "all"
			if changed.Load() {
				policy = "collaborators_only"
			}
			fmt.Fprintf(w, `{"has_pull_requests":true,"pull_request_creation_policy":%q,"private":true,"temp_clone_token":"unmanaged"}`, policy)
		case http.MethodPatch:
			var body map[string]any
			expected := map[string]any{"has_pull_requests": true, "pull_request_creation_policy": "collaborators_only"}
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil || !reflect.DeepEqual(body, expected) {
				t.Errorf("Unexpected payload %v: %v", body, err)
			}
			changed.Store(true)
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("Unexpected method %s", req.Method)
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	actual, err := policy.sync(context.Background(), policyIdentity{owner: "owner", repository: "repo"}, map[string]attr.Value{
		"has_pull_requests":            types.BoolValue(true),
		"pull_request_creation_policy": types.StringValue("collaborators_only"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !changed.Load() || len(actual.Attributes()) != 4 {
		t.Fatalf("Expected the two policy fields and repository identity: %v", actual.AttributeTypes(context.Background()))
	}
}

func TestImmutableReleaseDisableUsesDelete(t *testing.T) {
	var enabled atomic.Bool
	enabled.Store(true)
	policy := testPolicy(t, "repository_immutable_releases", func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/repos/owner/repo/immutable-releases" {
			t.Errorf("Unexpected path %s", req.URL.Path)
		}
		switch req.Method {
		case http.MethodGet:
			fmt.Fprintf(w, `{"enabled":%t,"enforced_by_owner":false}`, enabled.Load())
		case http.MethodDelete:
			enabled.Store(false)
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("Unexpected method %s", req.Method)
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	_, err := policy.sync(context.Background(), policyIdentity{owner: "owner", repository: "repo"}, map[string]attr.Value{"enabled": types.BoolValue(false)})
	if err != nil || enabled.Load() {
		t.Fatalf("Expected disabled immutable releases: %v", err)
	}
}

func TestImmutableReleaseUnknownDoesNotWrite(t *testing.T) {
	policy := testPolicy(t, "repository_immutable_releases", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet {
			t.Errorf("Unexpected method %s", req.Method)
		}
		fmt.Fprint(w, `{"enabled":true,"enforced_by_owner":false}`)
	})
	_, err := policy.sync(context.Background(), policyIdentity{owner: "owner", repository: "repo"}, map[string]attr.Value{"enabled": types.BoolUnknown()})
	if err == nil {
		t.Fatal("Expected an error for the unknown desired value")
	}
}

func TestImmutableRelease404RequiresAdminConfirmation(t *testing.T) {
	for _, admin := range []bool{true, false} {
		t.Run(fmt.Sprint(admin), func(t *testing.T) {
			policy := testPolicy(t, "repository_immutable_releases", func(w http.ResponseWriter, req *http.Request) {
				switch req.URL.Path {
				case "/repos/owner/repo/immutable-releases":
					w.WriteHeader(http.StatusNotFound)
				case "/repos/owner/repo":
					fmt.Fprintf(w, `{"permissions":{"admin":%t}}`, admin)
				default:
					t.Errorf("Unexpected path %s", req.URL.Path)
					w.WriteHeader(http.StatusBadRequest)
				}
			})
			result, err := policy.read(context.Background(), policyIdentity{owner: "owner", repository: "repo"})
			if admin && (err != nil || result.Attributes()["enabled"].(types.Bool).ValueBool()) {
				t.Fatalf("Expected confirmed disabled state: %v", err)
			}
			if !admin && err == nil {
				t.Fatal("Expected a permission failure")
			}
		})
	}
}

func TestOIDCSubjectTemplateKeepsOrderedClaims(t *testing.T) {
	var immutable atomic.Bool
	policy := testPolicy(t, "repository_oidc_subject_template", func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/repos/owner/repo/actions/oidc/customization/sub" {
			t.Errorf("Unexpected path %s", req.URL.Path)
		}
		switch req.Method {
		case http.MethodGet:
			fmt.Fprintf(w, `{"use_default":false,"use_immutable_subject":%t,"include_claim_keys":["environment","repo"],"sub_claim_prefix":"server-generated"}`, immutable.Load())
		case http.MethodPut:
			var payload map[string]any
			expected := map[string]any{
				"use_default": false, "use_immutable_subject": true,
				"include_claim_keys": []any{"environment", "repo"},
			}
			if err := json.NewDecoder(req.Body).Decode(&payload); err != nil || !reflect.DeepEqual(payload, expected) {
				t.Errorf("Unexpected OIDC payload %v, %v", payload, err)
			}
			immutable.Store(true)
			w.WriteHeader(http.StatusCreated)
		default:
			t.Errorf("Unexpected method %s", req.Method)
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	ctx := context.Background()
	keys, diags := types.ListValueFrom(ctx, types.StringType, []string{"environment", "repo"})
	if diags.HasError() {
		t.Fatal(diags)
	}
	actual, err := policy.sync(ctx, policyIdentity{owner: "owner", repository: "repo"}, map[string]attr.Value{
		"use_default": types.BoolValue(false), "use_immutable_subject": types.BoolValue(true),
		"include_claim_keys": keys,
	})
	if err != nil || !immutable.Load() || !actual.Attributes()["include_claim_keys"].Equal(keys) {
		t.Fatalf("Expected the original ordered claims and immutable subjects: %v", err)
	}
}

func TestSecretScanningPatchOnlyChangesNonProviderPatterns(t *testing.T) {
	var enabled atomic.Bool
	policy := testPolicy(t, "repository_secret_scanning_non_provider_patterns", func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/repos/owner/repo" {
			t.Errorf("Unexpected path %s", req.URL.Path)
		}
		switch req.Method {
		case http.MethodGet:
			status := "disabled"
			if enabled.Load() {
				status = "enabled"
			}
			fmt.Fprintf(w, `{"security_and_analysis":{"secret_scanning":{"status":"enabled"},"secret_scanning_validity_checks":{"status":"disabled"},"secret_scanning_non_provider_patterns":{"status":%q}}}`, status)
		case http.MethodPatch:
			var payload map[string]any
			expected := map[string]any{"security_and_analysis": map[string]any{
				"secret_scanning_non_provider_patterns": map[string]any{"status": "enabled"},
			}}
			if err := json.NewDecoder(req.Body).Decode(&payload); err != nil || !reflect.DeepEqual(payload, expected) {
				t.Errorf("Unexpected secret scanning payload %v, %v", payload, err)
			}
			enabled.Store(true)
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("Unexpected method %s", req.Method)
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	_, err := policy.sync(context.Background(), policyIdentity{owner: "owner", repository: "repo"}, map[string]attr.Value{"status": types.StringValue("enabled")})
	if err != nil || !enabled.Load() {
		t.Fatalf("Expected enabled non-provider pattern scanning: %v", err)
	}
}

func TestArchivedSettingsAreReadOnly(t *testing.T) {
	current := map[string]any{
		"archived":         true,
		"allow_auto_merge": false, "allow_forking": true,
		"allow_merge_commit": true, "allow_rebase_merge": true, "allow_squash_merge": true,
		"allow_update_branch": false, "delete_branch_on_merge": false,
		"merge_commit_message": "PR_TITLE", "merge_commit_title": "MERGE_MESSAGE",
		"squash_merge_commit_message": "COMMIT_MESSAGES", "squash_merge_commit_title": "COMMIT_OR_PR_TITLE",
		"web_commit_signoff_required": false,
	}
	policy := testPolicy(t, "repository_archived_settings", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet || req.URL.Path != "/repos/owner/repo" {
			t.Errorf("Unexpected request %s %s", req.Method, req.URL.Path)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if err := json.NewEncoder(w).Encode(current); err != nil {
			t.Errorf("Encode repository: %v", err)
		}
	})
	ctx := context.Background()
	identity := policyIdentity{owner: "owner", repository: "repo"}
	state, err := policy.read(ctx, identity)
	if err != nil {
		t.Fatal(err)
	}
	values := state.Attributes()
	if !values["archived"].(types.Bool).ValueBool() {
		t.Fatal("Expected to read the archive status")
	}
	if _, err := policy.sync(ctx, identity, values); err != nil {
		t.Fatalf("Expected matching archived settings: %v", err)
	}
	values["delete_branch_on_merge"] = types.BoolValue(true)
	if _, err := policy.sync(ctx, identity, values); err == nil {
		t.Fatal("Expected an error for mismatched archived settings")
	}
}

func TestSecretScanningValidityChecksAreReadOnly(t *testing.T) {
	var reads atomic.Int32
	policy := testPolicy(t, "repository_secret_scanning_validity_checks", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet || req.URL.Path != "/repos/owner/repo" {
			t.Errorf("Unexpected request %s %s", req.Method, req.URL.Path)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		reads.Add(1)
		fmt.Fprint(w, `{"security_and_analysis":{"secret_scanning":{"status":"enabled"},"secret_scanning_validity_checks":{"status":"disabled"}}}`)
	})
	ctx := context.Background()
	identity := policyIdentity{owner: "owner", repository: "repo"}
	state, err := policy.read(ctx, identity)
	if err != nil {
		t.Fatal(err)
	}
	values := state.Attributes()
	if got := values["status"].(types.String).ValueString(); got != "disabled" {
		t.Fatalf("Expected the disabled validity checks status, got %q", got)
	}
	if _, err := policy.sync(ctx, identity, values); err != nil {
		t.Fatalf("Expected matching validity checks: %v", err)
	}
	values["status"] = types.StringValue("enabled")
	if _, err := policy.sync(ctx, identity, values); err == nil {
		t.Fatal("Expected an error for mismatched validity checks")
	}
	if reads.Load() != 3 {
		t.Fatalf("Expected three reads and no writes, got %d requests", reads.Load())
	}
}

func TestOrganizationRecordedSettingsAreReadOnly(t *testing.T) {
	current := map[string]any{
		"default_repository_branch":                "trunk",
		"members_can_change_repo_visibility":       true,
		"members_can_create_teams":                 true,
		"members_can_delete_issues":                false,
		"members_can_delete_repositories":          false,
		"members_can_invite_outside_collaborators": true,
		"members_can_view_dependency_insights":     true,
		"readers_can_create_discussions":           true,
		"secret_scanning_validity_checks_enabled":  false,
		"two_factor_requirement_enabled":           true,
		"name":                                     "example-organization",
		"billing_email":                            "ignored@example.com",
	}
	policy := testPolicy(t, "organization_recorded_settings", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet || req.URL.Path != "/orgs/example-organization" {
			t.Errorf("Unexpected request %s %s", req.Method, req.URL.Path)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if err := json.NewEncoder(w).Encode(current); err != nil {
			t.Errorf("Encode organization: %v", err)
		}
	})
	ctx := context.Background()
	identity := policyIdentity{owner: "example-organization"}
	state, err := policy.read(ctx, identity)
	if err != nil {
		t.Fatal(err)
	}
	values := state.Attributes()
	if got := values["default_repository_branch"].(types.String).ValueString(); got != "trunk" {
		t.Fatalf("Expected the recorded repository branch, got %q", got)
	}
	if _, err := policy.sync(ctx, identity, values); err != nil {
		t.Fatalf("Expected matching organization settings: %v", err)
	}
	values["default_repository_branch"] = types.StringValue("main")
	if _, err := policy.sync(ctx, identity, values); err == nil {
		t.Fatal("Expected an error for mismatched organization settings")
	}
}

func TestPolicies(t *testing.T) {
	names := make(map[string]bool, len(policies))
	readOnly := 0
	for _, spec := range policies {
		if names[spec.name] {
			t.Errorf("Duplicate policy %q", spec.name)
		}
		names[spec.name] = true
		if spec.readOnly {
			readOnly++
		}
	}
	if len(policies) != 12 || readOnly != 3 {
		t.Fatalf("Expected 12 policies with 3 read-only, got %d with %d read-only", len(policies), readOnly)
	}
	for _, name := range []string{"repository_actions_access_level", "repository_workflow_permissions"} {
		if names[name] {
			t.Fatalf("%s is provided upstream", name)
		}
	}
}

func TestConfigValidatorChecksKnownPolicyFieldsWithUnknownTarget(t *testing.T) {
	ctx := context.Background()
	for _, testCase := range []struct {
		name     string
		values   map[string]attr.Value
		expected string
	}{
		{
			name: repositoryOIDCSubjectTemplate,
			values: map[string]attr.Value{
				"owner":                 types.StringUnknown(),
				"repository":            types.StringUnknown(),
				"use_default":           types.BoolValue(true),
				"use_immutable_subject": types.BoolValue(false),
				"include_claim_keys":    stringList(t, []string{"repo"}),
			},
			expected: "Omit include_claim_keys when use_default is true.",
		},
		{
			name: organizationImmutableReleases,
			values: map[string]attr.Value{
				"owner":                   types.StringUnknown(),
				"enforced_repositories":   types.StringValue("selected"),
				"selected_repository_ids": types.SetNull(types.Int64Type),
			},
			expected: "Set selected_repository_ids when enforced_repositories is selected and omit it otherwise.",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			spec := testPolicySpec(t, testCase.name)
			validator := policyConfigValidator{spec: spec}
			var schemaResponse resource.SchemaResponse
			(&policyResource{spec: spec}).Schema(ctx, resource.SchemaRequest{}, &schemaResponse)
			attributeTypes := map[string]attr.Type{"owner": types.StringType}
			if !spec.organization {
				attributeTypes["repository"] = types.StringType
			}
			for _, field := range spec.fields {
				attributeTypes[field.name] = field.typ
			}
			object, diagnostics := types.ObjectValue(attributeTypes, testCase.values)
			if diagnostics.HasError() {
				t.Fatal(diagnostics)
			}
			raw, err := object.ToTerraformValue(ctx)
			if err != nil {
				t.Fatal(err)
			}
			request := resource.ValidateConfigRequest{Config: tfsdk.Config{Schema: schemaResponse.Schema, Raw: raw}}
			response := resource.ValidateConfigResponse{}
			validator.ValidateResource(ctx, request, &response)
			if !response.Diagnostics.HasError() {
				t.Fatal("Expected a policy validation error with an unknown target")
			}
			if got := response.Diagnostics.Errors()[0].Detail(); got != testCase.expected {
				t.Fatalf("Unexpected policy validation error %q", got)
			}
		})
	}
}

func TestOIDCSubjectTemplateClaimValidation(t *testing.T) {
	ctx := context.Background()
	spec := testPolicySpec(t, repositoryOIDCSubjectTemplate)
	validator := policyConfigValidator{spec: spec}
	var schemaResponse resource.SchemaResponse
	(&policyResource{spec: spec}).Schema(ctx, resource.SchemaRequest{}, &schemaResponse)
	attributeTypes := map[string]attr.Type{
		"owner":                 types.StringType,
		"repository":            types.StringType,
		"use_default":           types.BoolType,
		"use_immutable_subject": types.BoolType,
		"include_claim_keys":    types.ListType{ElemType: types.StringType},
	}
	for _, testCase := range []struct {
		name     string
		keys     types.List
		rejected bool
	}{
		{
			name: "known and unknown",
			keys: types.ListValueMust(types.StringType, []attr.Value{
				types.StringValue("environment"),
				types.StringUnknown(),
			}),
		},
		{
			name: "unknown only",
			keys: types.ListValueMust(types.StringType, []attr.Value{types.StringUnknown()}),
		},
		{
			name:     "invalid known",
			keys:     stringList(t, []string{"environment", "not a claim"}),
			rejected: true,
		},
		{
			name:     "duplicate known",
			keys:     stringList(t, []string{"environment", "environment"}),
			rejected: true,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			object, diagnostics := types.ObjectValue(attributeTypes, map[string]attr.Value{
				"owner":                 types.StringValue("owner"),
				"repository":            types.StringValue("repo"),
				"use_default":           types.BoolValue(false),
				"use_immutable_subject": types.BoolValue(false),
				"include_claim_keys":    testCase.keys,
			})
			if diagnostics.HasError() {
				t.Fatal(diagnostics)
			}
			raw, err := object.ToTerraformValue(ctx)
			if err != nil {
				t.Fatal(err)
			}
			request := resource.ValidateConfigRequest{Config: tfsdk.Config{Schema: schemaResponse.Schema, Raw: raw}}
			response := resource.ValidateConfigResponse{}
			validator.ValidateResource(ctx, request, &response)
			if response.Diagnostics.HasError() != testCase.rejected {
				t.Fatalf("Unexpected diagnostics: %v", response.Diagnostics)
			}
		})
	}
}

func TestOIDCSubjectTemplateNormalizesDefaultClaims(t *testing.T) {
	policy := testPolicy(t, repositoryOIDCSubjectTemplate, func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet || req.URL.Path != "/repos/owner/repo/actions/oidc/customization/sub" {
			t.Errorf("Unexpected request %s %s", req.Method, req.URL.Path)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		fmt.Fprint(w, `{"use_default":true,"use_immutable_subject":false,"include_claim_keys":["repo"]}`)
	})
	identity := policyIdentity{owner: "owner", repository: "repo"}
	desired := map[string]attr.Value{
		"use_default":           types.BoolValue(true),
		"use_immutable_subject": types.BoolValue(false),
		"include_claim_keys":    types.ListNull(types.StringType),
	}
	actual, err := policy.sync(context.Background(), identity, desired)
	if err != nil {
		t.Fatal(err)
	}
	if !actual.Attributes()["include_claim_keys"].(types.List).IsNull() {
		t.Fatal("Expected the ignored claim keys to be omitted from state")
	}
}

func TestOIDCNormalizationDoesNotDependOnFieldOrder(t *testing.T) {
	spec := testPolicySpec(t, repositoryOIDCSubjectTemplate)
	spec.fields = []policyField{spec.fields[2], spec.fields[0], spec.fields[1]}
	policy := testPolicyWithSpec(t, spec, func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet {
			t.Errorf("Unexpected request %s %s", req.Method, req.URL.Path)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		fmt.Fprint(w, `{"use_default":true,"use_immutable_subject":false,"include_claim_keys":["repo"]}`)
	})
	actual, err := policy.read(context.Background(), policyIdentity{owner: "owner", repository: "repo"})
	if err != nil {
		t.Fatal(err)
	}
	if !actual.Attributes()["include_claim_keys"].(types.List).IsNull() {
		t.Fatal("Expected default claims to be normalized regardless of field order")
	}
}

func TestOrganizationImmutableReleasesSelectedRepositories(t *testing.T) {
	const count = 105
	ids := make([]int64, count)
	for index := range ids {
		ids[index] = int64(index + 1)
	}
	var mutex sync.Mutex
	remoteMode := "none"
	remoteIDs := []int64{}
	puts := 0
	policy := testPolicy(t, organizationImmutableReleases, func(w http.ResponseWriter, req *http.Request) {
		switch {
		case req.Method == http.MethodGet && req.URL.Path == "/orgs/owner/settings/immutable-releases":
			mutex.Lock()
			mode := remoteMode
			mutex.Unlock()
			fmt.Fprintf(w, `{"enforced_repositories":%q}`, mode)
		case req.Method == http.MethodGet && req.URL.Path == "/orgs/owner/settings/immutable-releases/repositories":
			mutex.Lock()
			mode := remoteMode
			current := append([]int64{}, remoteIDs...)
			mutex.Unlock()
			if mode != "selected" {
				t.Errorf("Unexpected selected repository request while mode is %q", mode)
			}
			page, err := strconv.Atoi(req.URL.Query().Get("page"))
			if err != nil {
				t.Errorf("Invalid page: %v", err)
			}
			start := (page - 1) * 100
			if start > len(current) {
				start = len(current)
			}
			end := min(start+100, len(current))
			repositories := make([]map[string]int64, 0, end-start)
			for _, id := range current[start:end] {
				repositories = append(repositories, map[string]int64{"id": id})
			}
			if err := json.NewEncoder(w).Encode(map[string]any{"total_count": len(current), "repositories": repositories}); err != nil {
				t.Errorf("Encode selected repositories: %v", err)
			}
		case req.Method == http.MethodPut && req.URL.Path == "/orgs/owner/settings/immutable-releases":
			var body struct {
				Mode string  `json:"enforced_repositories"`
				IDs  []int64 `json:"selected_repository_ids"`
			}
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				t.Errorf("Decode immutable releases payload: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			got := append([]int64{}, body.IDs...)
			want := append([]int64{}, ids...)
			slices.Sort(got)
			slices.Sort(want)
			if body.Mode != "selected" || !reflect.DeepEqual(got, want) {
				t.Errorf("Unexpected immutable releases payload %+v", body)
			}
			mutex.Lock()
			remoteMode = body.Mode
			remoteIDs = append([]int64{}, body.IDs...)
			puts++
			mutex.Unlock()
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("Unexpected request %s %s", req.Method, req.URL.Path)
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	ctx := context.Background()
	identity := policyIdentity{owner: "owner"}
	selected, diagnostics := types.SetValueFrom(ctx, types.Int64Type, ids)
	if diagnostics.HasError() {
		t.Fatal(diagnostics)
	}
	desired := map[string]attr.Value{
		"enforced_repositories":   types.StringValue("selected"),
		"selected_repository_ids": selected,
	}
	actual, err := policy.sync(ctx, identity, desired)
	if err != nil {
		t.Fatal(err)
	}
	writeCount := func() int {
		mutex.Lock()
		defer mutex.Unlock()
		return puts
	}
	if writeCount() != 1 {
		t.Fatalf("Expected one write, got %d", writeCount())
	}
	var actualIDs []int64
	if diagnostics := actual.Attributes()["selected_repository_ids"].(types.Set).ElementsAs(ctx, &actualIDs, false); diagnostics.HasError() {
		t.Fatal(diagnostics)
	}
	slices.Sort(actualIDs)
	if !reflect.DeepEqual(actualIDs, ids) {
		t.Fatalf("Expected all selected repository IDs, got %d", len(actualIDs))
	}
	if _, err := policy.sync(ctx, identity, desired); err != nil {
		t.Fatal(err)
	}
	if writeCount() != 1 {
		t.Fatalf("Expected no write for a matching configuration, got %d writes", writeCount())
	}
}

func TestOrganizationImmutableReleasesRejectsInvalidSelectedRepositories(t *testing.T) {
	for _, testCase := range []struct {
		name         string
		totalCount   int
		repositories []int64
	}{
		{name: "duplicate", totalCount: 2, repositories: []int64{1, 1}},
		{name: "mismatched count", totalCount: 2, repositories: []int64{1}},
		{name: "invalid ID", totalCount: 1, repositories: []int64{0}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			policy := testPolicy(t, organizationImmutableReleases, func(w http.ResponseWriter, req *http.Request) {
				switch req.URL.Path {
				case "/orgs/owner/settings/immutable-releases":
					fmt.Fprint(w, `{"enforced_repositories":"selected"}`)
				case "/orgs/owner/settings/immutable-releases/repositories":
					repositories := make([]map[string]int64, 0, len(testCase.repositories))
					for _, id := range testCase.repositories {
						repositories = append(repositories, map[string]int64{"id": id})
					}
					if err := json.NewEncoder(w).Encode(map[string]any{"total_count": testCase.totalCount, "repositories": repositories}); err != nil {
						t.Errorf("Encode selected repositories: %v", err)
					}
				default:
					t.Errorf("Unexpected request %s %s", req.Method, req.URL.Path)
					w.WriteHeader(http.StatusMethodNotAllowed)
				}
			})
			if _, err := policy.read(context.Background(), policyIdentity{owner: "owner"}); err == nil {
				t.Fatal("Expected an inconsistent selected repository list to fail")
			}
		})
	}
}

func TestRateLimitStopsFurtherPolicyRequests(t *testing.T) {
	var requests atomic.Int64
	policy := testPolicy(t, "repository_pull_requests", func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests)
	})
	for range 2 {
		_, err := policy.read(context.Background(), policyIdentity{owner: "owner", repository: "repo"})
		if err == nil || !strings.Contains(err.Error(), "GitHub API rate limit reached; retry after") {
			t.Fatalf("Expected rate-limit guidance, got %v", err)
		}
	}
	if requests.Load() != 1 {
		t.Fatalf("Expected one request during cooldown, got %d", requests.Load())
	}
}

func TestSelectedRepositoriesRequireCompleteResponse(t *testing.T) {
	for _, reply := range []string{`{}`, `{"total_count":0}`, `{"total_count":0,"repositories":[]}{}`} {
		policy := testPolicy(t, organizationImmutableReleases, func(w http.ResponseWriter, req *http.Request) {
			if strings.HasSuffix(req.URL.Path, "/repositories") {
				fmt.Fprint(w, reply)
				return
			}
			fmt.Fprint(w, `{"enforced_repositories":"selected"}`)
		})
		if _, err := policy.read(context.Background(), policyIdentity{owner: "owner"}); err == nil {
			t.Fatalf("Expected invalid selected repository response to fail: %s", reply)
		}
	}
}

func TestImmutableReleaseNoOpDoesNotWrite(t *testing.T) {
	var writes atomic.Int32
	policy := testPolicy(t, repositoryImmutableReleases, func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet {
			writes.Add(1)
			t.Errorf("Unexpected write %s", req.Method)
			return
		}
		fmt.Fprint(w, `{"enabled":true,"enforced_by_owner":false}`)
	})
	if _, err := policy.sync(context.Background(), policyIdentity{owner: "owner", repository: "repo"}, map[string]attr.Value{"enabled": types.BoolValue(true)}); err != nil {
		t.Fatal(err)
	}
	if writes.Load() != 0 {
		t.Fatalf("Expected no remote writes, got %d", writes.Load())
	}
}

func TestImmutableReleaseEnforcedByOwnerRejectsWrite(t *testing.T) {
	policy := testPolicy(t, repositoryImmutableReleases, func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet {
			t.Errorf("Unexpected write %s", req.Method)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		fmt.Fprint(w, `{"enabled":true,"enforced_by_owner":true}`)
	})
	_, err := policy.sync(context.Background(), policyIdentity{owner: "owner", repository: "repo"}, map[string]attr.Value{"enabled": types.BoolValue(false)})
	if err == nil || !strings.Contains(err.Error(), "enforces immutable releases") {
		t.Fatalf("Expected owner enforcement to reject the change, got %v", err)
	}
}

func TestPolicyDeleteDoesNotWrite(t *testing.T) {
	var requests atomic.Int32
	policy := testPolicy(t, "repository_fork_contributor_approval", func(w http.ResponseWriter, req *http.Request) {
		requests.Add(1)
		t.Errorf("Unexpected request %s %s", req.Method, req.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	})
	ctx := context.Background()
	var schemaResponse resource.SchemaResponse
	policy.Schema(ctx, resource.SchemaRequest{}, &schemaResponse)
	state := tfsdk.State{Schema: schemaResponse.Schema}
	object, diagnostics := types.ObjectValue(map[string]attr.Type{
		"owner":           types.StringType,
		"repository":      types.StringType,
		"approval_policy": types.StringType,
	}, map[string]attr.Value{
		"owner":           types.StringValue("owner"),
		"repository":      types.StringValue("repo"),
		"approval_policy": types.StringValue("first_time_contributors"),
	})
	if diagnostics.HasError() {
		t.Fatal(diagnostics)
	}
	if diagnostics := state.Set(ctx, object); diagnostics.HasError() {
		t.Fatal(diagnostics)
	}
	response := resource.DeleteResponse{State: state}
	policy.Delete(ctx, resource.DeleteRequest{State: state}, &response)
	if requests.Load() != 0 {
		t.Fatalf("Expected no remote requests, got %d", requests.Load())
	}
	if !response.State.Raw.IsNull() {
		t.Fatal("Expected the resource to be removed from state")
	}
}

func stringList(t *testing.T, values []string) types.List {
	t.Helper()
	list, diagnostics := types.ListValueFrom(context.Background(), types.StringType, values)
	if diagnostics.HasError() {
		t.Fatal(diagnostics)
	}
	return list
}
