resource "githubsettings_repository_private_fork_workflows" "example" {
  owner                                  = "example-owner"
  repository                             = "example-repository"
  run_workflows_from_fork_pull_requests  = false
  send_write_tokens_to_workflows         = false
  send_secrets_and_variables             = false
  require_approval_for_fork_pr_workflows = false
}
