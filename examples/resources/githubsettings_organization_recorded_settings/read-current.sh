GH_TOKEN="${GITHUB_TOKEN:?Set GITHUB_TOKEN first}" gh api --hostname github.com orgs/OWNER \
  -H "X-GitHub-Api-Version: 2026-03-10" \
  --jq '{
  default_repository_branch,
  members_can_change_repo_visibility,
  members_can_create_teams,
  members_can_delete_issues,
  members_can_delete_repositories,
  members_can_invite_outside_collaborators,
  members_can_view_dependency_insights,
  readers_can_create_discussions,
  secret_scanning_validity_checks_enabled,
  two_factor_requirement_enabled
}'
