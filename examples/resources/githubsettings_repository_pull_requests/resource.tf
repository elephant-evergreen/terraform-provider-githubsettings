resource "githubsettings_repository_pull_requests" "example" {
  owner                        = "example-owner"
  repository                   = "example-repository"
  has_pull_requests            = true
  pull_request_creation_policy = "all"
}
