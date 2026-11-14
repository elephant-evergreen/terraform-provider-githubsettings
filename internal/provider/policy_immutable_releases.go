package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"net/http"
)

var repositoryImmutableReleasesPolicy = policySpec{
	name:         repositoryImmutableReleases,
	suffix:       "/immutable-releases",
	readFallback: repositoryImmutableReleasesReply,
	write:        repositoryImmutableReleasesWrite,
	description:  "Manages immutable releases for one repository. Creating or updating this resource applies the configured policy to GitHub; destroying it removes the resource from Terraform state without changing the policy. GitHub can enforce immutable releases through the repository owner. enforced_by_owner reports that policy and the provider rejects changes while it is active. Requires a token with repository administration write access.",
	fields: []policyField{
		flag("enabled", "Enable immutable releases."),
		{name: "enforced_by_owner", typ: types.BoolType, computed: true, description: "Whether the repository owner enforces immutable releases. The provider rejects changes while this is true."},
	},
}

var organizationImmutableReleasesPolicy = policySpec{
	name:         organizationImmutableReleases,
	organization: true,
	suffix:       "/settings/immutable-releases",
	method:       http.MethodPut,
	validate:     organizationImmutableReleasesRule,
	normalize:    readOrganizationImmutableReleases,
	description:  "Manages immutable releases for repositories in one organization. Creating or updating this resource applies the configured policy to GitHub; destroying it removes the resource from Terraform state without changing it. Requires a token with organization administration write access.",
	fields: []policyField{
		choice("enforced_repositories", "Repositories with immutable releases.", "none", "all", "selected"),
		{name: "selected_repository_ids", typ: types.SetType{ElemType: types.Int64Type}, description: "Repository IDs with immutable releases. Required when enforced_repositories is selected."},
	},
}

func organizationImmutableReleasesRule(_ context.Context, values map[string]attr.Value) error {
	policy, ok := values["enforced_repositories"].(types.String)
	if !ok || policy.IsNull() || policy.IsUnknown() {
		return nil
	}
	ids, ok := values["selected_repository_ids"].(types.Set)
	if !ok || ids.IsUnknown() {
		return nil
	}
	if (policy.ValueString() == "selected") != !ids.IsNull() {
		return &policyRuleError{attribute: "selected_repository_ids", message: "Set selected_repository_ids when enforced_repositories is selected and omit it otherwise."}
	}
	for _, id := range ids.Elements() {
		value := id.(types.Int64)
		if value.IsNull() || (!value.IsUnknown() && value.ValueInt64() <= 0) {
			return &policyRuleError{attribute: "selected_repository_ids", message: "Selected repository IDs must be positive integers."}
		}
	}
	return nil
}

func repositoryImmutableReleasesWrite(actual, desired map[string]attr.Value) (string, any, error) {
	enforced, ok := actual["enforced_by_owner"].(types.Bool)
	if !ok || enforced.IsNull() || enforced.IsUnknown() {
		return "", nil, errors.New("GitHub did not report whether the repository owner enforces immutable releases")
	}
	if enforced.ValueBool() {
		return "", nil, errors.New("GitHub enforces immutable releases through the repository owner")
	}
	enabled, ok := desired["enabled"].(types.Bool)
	if !ok || enabled.IsNull() || enabled.IsUnknown() {
		return "", nil, errors.New("enabled is unknown")
	}
	if enabled.ValueBool() {
		return http.MethodPut, nil, nil
	}
	return http.MethodDelete, nil, nil
}

func repositoryImmutableReleasesReply(ctx context.Context, client *gitHubClient, identity policyIdentity, readErr error) (map[string]json.RawMessage, error) {
	var status *statusError
	if !errors.As(readErr, &status) || status.status != http.StatusNotFound {
		return nil, readErr
	}
	var repository struct{ Permissions struct{ Admin bool } }
	if err := client.request(ctx, http.MethodGet, "/repos/"+identity.owner+"/"+identity.repository, nil, &repository); err != nil {
		return nil, err
	}
	if !repository.Permissions.Admin {
		return nil, readErr
	}
	return map[string]json.RawMessage{"enabled": json.RawMessage("false"), "enforced_by_owner": json.RawMessage("false")}, nil
}

func readOrganizationImmutableReleases(ctx context.Context, client *gitHubClient, endpoint string, fields map[string]attr.Value) error {
	policy, ok := fields["enforced_repositories"].(types.String)
	if !ok || policy.IsNull() || policy.IsUnknown() || policy.ValueString() != "selected" {
		return nil
	}
	ids, err := readSelectedRepositories(ctx, client, endpoint)
	if err != nil {
		return err
	}
	value, diagnostics := types.SetValueFrom(ctx, types.Int64Type, ids)
	if diagnostics.HasError() {
		return errors.New("could not read selected repository IDs")
	}
	fields["selected_repository_ids"] = value
	return nil
}

func readSelectedRepositories(ctx context.Context, client *gitHubClient, endpoint string) ([]int64, error) {
	ids := make([]int64, 0)
	seen := map[int64]bool{}
	total := 0
	for page := 1; ; page++ {
		var response struct {
			TotalCount   *int                 `json:"total_count"`
			Repositories []struct{ ID int64 } `json:"repositories"`
		}
		path := fmt.Sprintf("%s/repositories?per_page=100&page=%d", endpoint, page)
		if err := client.request(ctx, http.MethodGet, path, nil, &response); err != nil {
			return nil, err
		}
		if response.TotalCount == nil || *response.TotalCount < 0 || response.Repositories == nil || len(response.Repositories) > 100 {
			return nil, errors.New("GitHub returned an incomplete selected repository page")
		}
		if page == 1 {
			total = *response.TotalCount
		}
		if *response.TotalCount != total || len(ids)+len(response.Repositories) > total {
			return nil, errors.New("selected repositories changed during pagination")
		}
		for _, repository := range response.Repositories {
			if repository.ID <= 0 || seen[repository.ID] {
				return nil, errors.New("GitHub returned an invalid or duplicate selected repository ID")
			}
			seen[repository.ID] = true
			ids = append(ids, repository.ID)
		}
		if len(ids) == total {
			return ids, nil
		}
		if len(response.Repositories) < 100 {
			return nil, errors.New("selected repositories changed during pagination")
		}
	}
}
