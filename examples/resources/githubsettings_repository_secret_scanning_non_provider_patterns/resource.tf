resource "githubsettings_repository_secret_scanning_non_provider_patterns" "example" {
  owner      = "example-owner"
  repository = "example-repository"
  status     = "enabled"
}
