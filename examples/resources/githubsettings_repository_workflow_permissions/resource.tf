resource "githubsettings_repository_workflow_permissions" "example" {
  owner                            = "example-owner"
  repository                       = "example-repository"
  default_workflow_permissions     = "read"
  can_approve_pull_request_reviews = false
}
