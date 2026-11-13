resource "githubsettings_repository_secret_scanning_validity_checks" "example" {
  owner      = "example-owner"
  repository = "example-repository"
  status     = "enabled"
}
