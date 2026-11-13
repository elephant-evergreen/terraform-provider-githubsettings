resource "githubsettings_repository_fork_contributor_approval" "example" {
  owner           = "example-owner"
  repository      = "example-repository"
  approval_policy = "first_time_contributors"
}
