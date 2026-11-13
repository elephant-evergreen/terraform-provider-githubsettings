resource "githubsettings_organization_immutable_releases" "example" {
  owner                   = "example-owner"
  enforced_repositories   = "selected"
  selected_repository_ids = [123456789]
}
