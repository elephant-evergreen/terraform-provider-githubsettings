resource "githubsettings_organization_deploy_keys" "example" {
  owner                                = "example-owner"
  deploy_keys_enabled_for_repositories = true
}
