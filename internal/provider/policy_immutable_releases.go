package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var repositoryImmutableReleasesPolicy = policySpec{
	name:        repositoryImmutableReleases,
	suffix:      "/immutable-releases",
	description: "Manages immutable releases for one repository. Creating or updating this resource applies the configured policy to GitHub; destroying it removes the resource from Terraform state without changing the policy. GitHub can enforce immutable releases through the repository owner. enforced_by_owner reports that policy and the provider rejects changes while it is active. Requires a token with repository administration write access.",
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
	description:  "Manages immutable releases for repositories in one organization. Creating or updating this resource applies the configured policy to GitHub; destroying it removes the resource from Terraform state without changing it. Requires a token with organization administration write access.",
	fields: []policyField{
		choice("enforced_repositories", "Repositories with immutable releases.", "none", "all", "selected"),
		{name: "selected_repository_ids", typ: types.SetType{ElemType: types.Int64Type}, description: "Repository IDs with immutable releases. Required when enforced_repositories is selected."},
	},
}

func organizationImmutableReleasesRule(values map[string]attr.Value) error {
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

func (r *policyResource) repositoryImmutableReleasesReply(ctx context.Context, identity policyIdentity, readErr error) (map[string]json.RawMessage, error) {
	var status *statusError
	if !errors.As(readErr, &status) || status.status != http.StatusNotFound {
		return nil, readErr
	}
	var repository struct{ Permissions struct{ Admin bool } }
	if err := r.client.request(ctx, http.MethodGet, "/repos/"+identity.owner+"/"+identity.repository, nil, &repository); err != nil {
		return nil, err
	}
	if !repository.Permissions.Admin {
		return nil, readErr
	}
	return map[string]json.RawMessage{"enabled": json.RawMessage("false"), "enforced_by_owner": json.RawMessage("false")}, nil
}

func (r *policyResource) readOrganizationImmutableReleases(ctx context.Context, endpoint string, fields map[string]attr.Value) error {
	policy, ok := fields["enforced_repositories"].(types.String)
	if !ok || policy.IsNull() || policy.IsUnknown() || policy.ValueString() != "selected" {
		return nil
	}
	ids, err := r.readSelectedRepositories(ctx, endpoint)
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

func (r *policyResource) readSelectedRepositories(ctx context.Context, endpoint string) ([]int64, error) {
	ids := make([]int64, 0)
	for page := 1; ; page++ {
		var response struct {
			TotalCount   int                  `json:"total_count"`
			Repositories []struct{ ID int64 } `json:"repositories"`
		}
		path := fmt.Sprintf("%s/repositories?per_page=100&page=%d", endpoint, page)
		if err := r.client.request(ctx, http.MethodGet, path, nil, &response); err != nil {
			return nil, err
		}
		for _, repository := range response.Repositories {
			ids = append(ids, repository.ID)
		}
		if len(response.Repositories) < 100 {
			slices.Sort(ids)
			if len(ids) != response.TotalCount || len(slices.Compact(ids)) != len(ids) {
				return nil, errors.New("selected repositories changed during pagination")
			}
			return ids, nil
		}
	}
}
