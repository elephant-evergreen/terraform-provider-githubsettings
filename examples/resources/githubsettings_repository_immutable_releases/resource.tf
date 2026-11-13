resource "githubsettings_repository_immutable_releases" "example" {
  owner      = "example-owner"
  repository = "example-repository"
  enabled    = true
}
