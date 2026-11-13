package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func testPolicy(t *testing.T, name string, handler http.HandlerFunc) *policyResource {
	t.Helper()
	for _, spec := range policies {
		if spec.name != name {
			continue
		}
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
	t.Fatalf("Unknown policy %q", name)
	return nil
}

func TestWorkflowPolicySkipsMatchingWrite(t *testing.T) {
	policy := testPolicy(t, "repository_workflow_permissions", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet || req.URL.Path != "/repos/owner/repo/actions/permissions/workflow" {
			t.Errorf("Unexpected request %s %s", req.Method, req.URL.Path)
		}
		fmt.Fprint(w, `{"default_workflow_permissions":"read","can_approve_pull_request_reviews":false}`)
	})
	_, err := policy.sync(context.Background(), policyIdentity{owner: "owner", repository: "repo"}, map[string]attr.Value{
		"default_workflow_permissions":     types.StringValue("read"),
		"can_approve_pull_request_reviews": types.BoolValue(false),
	})
	if err != nil {
		t.Fatal(err)
	}
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
	if len(policies) != 13 || readOnly != 3 {
		t.Fatalf("Expected 13 policies with 3 read-only, got %d with %d read-only", len(policies), readOnly)
	}
	if names["repository_actions_access_level"] {
		t.Fatal("repository_actions_access_level is provided upstream")
	}
}
