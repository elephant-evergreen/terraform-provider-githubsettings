resource "githubsettings_repository_oidc_subject_template" "example" {
  owner                 = "example-owner"
  repository            = "example-repository"
  use_default           = false
  use_immutable_subject = true
  include_claim_keys    = ["repo", "context"]
}
