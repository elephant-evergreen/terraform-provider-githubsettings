package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type policyField struct {
	name        string
	typ         attr.Type
	choices     []string
	computed    bool
	description string
}

type policySpec struct {
	name         string
	description  string
	organization bool
	suffix       string
	method       string
	fields       []policyField
	jsonPath     []string
	readOnly     bool
	validate     func(context.Context, map[string]attr.Value) error
	normalize    func(context.Context, *gitHubClient, string, map[string]attr.Value) error
	readFallback func(context.Context, *gitHubClient, policyIdentity, error) (map[string]json.RawMessage, error)
	write        func(map[string]attr.Value, map[string]attr.Value) (string, any, error)
}

const (
	repositoryOIDCSubjectTemplate = "repository_oidc_subject_template"
	repositoryImmutableReleases   = "repository_immutable_releases"
	organizationImmutableReleases = "organization_immutable_releases"
)

type policyRuleError struct {
	attribute string
	message   string
}

func (e *policyRuleError) Error() string {
	return e.message
}

func flag(name, description string) policyField {
	return policyField{name: name, typ: types.BoolType, description: description}
}

func choice(name, description string, options ...string) policyField {
	return policyField{name: name, typ: types.StringType, choices: options, description: description}
}

func text(name, description string) policyField {
	return policyField{name: name, typ: types.StringType, description: description}
}

func (f policyField) accepts(value string) bool {
	if len(f.choices) == 0 {
		return value != ""
	}
	return slices.Contains(f.choices, value)
}

func (f policyField) describe() string {
	if len(f.choices) == 0 {
		return f.description
	}
	return f.description + " Allowed values are " + strings.Join(f.choices, ", ") + "."
}

var privateForkFields = []policyField{
	flag("run_workflows_from_fork_pull_requests", "Run workflows from fork pull requests."),
	flag("send_write_tokens_to_workflows", "Send write tokens to workflows from fork pull requests."),
	flag("send_secrets_and_variables", "Send secrets and variables to workflows from fork pull requests."),
	flag("require_approval_for_fork_pr_workflows", "Require approval before running workflows from fork pull requests."),
}

var policies = []policySpec{
	{
		name: "repository_archived_settings", readOnly: true,
		description: "Records the merge settings of an archived repository without changing them. Set each argument to the value GitHub reports. Apply fails while the configured values differ. Give the token read access to the repository.",
		fields: []policyField{
			{name: "archived", typ: types.BoolType, computed: true, description: "Whether GitHub reports the repository as archived."},
			flag("allow_auto_merge", "Allow auto-merge for pull requests."),
			flag("allow_forking", "Allow users to fork the repository."),
			flag("allow_merge_commit", "Allow merge commits."),
			flag("allow_rebase_merge", "Allow rebase merging."),
			flag("allow_squash_merge", "Allow squash merging."),
			flag("allow_update_branch", "Allow branches to be updated from their pull requests."),
			flag("delete_branch_on_merge", "Delete head branches after pull requests merge."),
			choice("merge_commit_message", "Merge commit message.", "PR_BODY", "PR_TITLE", "BLANK"),
			choice("merge_commit_title", "Merge commit title.", "PR_TITLE", "MERGE_MESSAGE"),
			choice("squash_merge_commit_message", "Squash merge commit message.", "PR_BODY", "COMMIT_MESSAGES", "BLANK"),
			choice("squash_merge_commit_title", "Squash merge commit title.", "PR_TITLE", "COMMIT_OR_PR_TITLE"),
			flag("web_commit_signoff_required", "Require contributors to sign off on web-based commits."),
		},
	},
	repositoryOIDCSubjectTemplatePolicy,
	{
		name: "repository_secret_scanning_non_provider_patterns", method: http.MethodPatch,
		jsonPath:    []string{"security_and_analysis", "secret_scanning_non_provider_patterns"},
		fields:      []policyField{choice("status", "Secret scanning status.", "enabled", "disabled")},
		description: "Manages secret scanning for non-provider patterns in one repository. Requires a token with repository administration write access.",
	},
	{
		name: "repository_secret_scanning_validity_checks", readOnly: true,
		description: "Records the current secret scanning validity check status for this repository without changing it. Set `status` to the value GitHub reports. Apply fails while the configured status differs. Use a token with repository Administration read access.",
		jsonPath:    []string{"security_and_analysis", "secret_scanning_validity_checks"},
		fields:      []policyField{choice("status", "Secret scanning validity check status reported by GitHub.", "enabled", "disabled")},
	},
	{
		name: "organization_deploy_keys", organization: true, method: http.MethodPatch,
		fields:      []policyField{flag("deploy_keys_enabled_for_repositories", "Allow organization members to add deploy keys to repositories.")},
		description: "Manages whether organization members can add deploy keys to repositories. Requires a token with organization administration write access.",
	},
	{
		name: "organization_recorded_settings", organization: true, readOnly: true,
		description: "Records the organization settings listed in the schema below without changing them. Set each argument to the value GitHub reports. Apply fails while the configured values differ. Use a token with organization Administration read access.",
		fields: []policyField{
			text("default_repository_branch", "Default branch name for new repositories."),
			flag("members_can_change_repo_visibility", "Allow members to change repository visibility."),
			flag("members_can_create_teams", "Allow members to create teams."),
			flag("members_can_delete_issues", "Allow members to delete issues."),
			flag("members_can_delete_repositories", "Allow members to delete repositories."),
			flag("members_can_invite_outside_collaborators", "Allow members to invite outside collaborators."),
			flag("members_can_view_dependency_insights", "Allow members to view dependency insights."),
			flag("readers_can_create_discussions", "Allow users with read access to create discussions."),
			flag("secret_scanning_validity_checks_enabled", "Whether secret scanning validity checks are enabled."),
			flag("two_factor_requirement_enabled", "Whether two-factor authentication is required."),
		},
	},
	{
		name: "repository_fork_contributor_approval", suffix: "/actions/permissions/fork-pr-contributor-approval", method: http.MethodPut,
		description: "Manage the approval policy for workflows from fork pull requests by external contributors in one repository. To inspect and import the policy, use a fine-grained token with repository Administration read access. To change the policy, grant Administration write access.",
		fields:      []policyField{choice("approval_policy", "Approval policy for external contributors.", "first_time_contributors_new_to_github", "first_time_contributors", "all_external_contributors")},
	},
	{
		name: "repository_private_fork_workflows", suffix: "/actions/permissions/fork-pr-workflows-private-repos", method: http.MethodPut,
		description: "Manages whether workflows from private fork pull requests run in one repository and whether they receive write tokens, secrets, and variables. Requires a token with repository administration write access.",
		fields:      privateForkFields,
	},
	{
		name: "organization_private_fork_workflows", organization: true, suffix: "/actions/permissions/fork-pr-workflows-private-repos", method: http.MethodPut,
		description: "Manages whether workflows from private fork pull requests run across an organization and whether they receive write tokens, secrets, and variables. Requires a token with organization administration write access.",
		fields:      privateForkFields,
	},
	{
		name: "repository_pull_requests", method: http.MethodPatch,
		description: "Manages whether pull requests are enabled in one repository and who can create them. Requires a token with repository administration write access.",
		fields: []policyField{
			flag("has_pull_requests", "Enable pull requests."),
			choice("pull_request_creation_policy", "Who can create pull requests. `all` allows any user with access to the repository; `collaborators_only` allows collaborators.", "all", "collaborators_only"),
		},
	},
	repositoryImmutableReleasesPolicy,
	organizationImmutableReleasesPolicy,
}
