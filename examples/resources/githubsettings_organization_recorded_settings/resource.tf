resource "githubsettings_organization_recorded_settings" "example" {
  owner                                    = "example-owner"
  default_repository_branch                = "main"
  members_can_change_repo_visibility       = false
  members_can_create_teams                 = false
  members_can_delete_issues                = false
  members_can_delete_repositories          = false
  members_can_invite_outside_collaborators = false
  members_can_view_dependency_insights     = false
  readers_can_create_discussions           = false
  secret_scanning_validity_checks_enabled  = false
  two_factor_requirement_enabled           = false
}
