resource "githubsettings_repository_archived_settings" "example" {
  owner      = "example-owner"
  repository = "example-repository"

  allow_auto_merge            = false
  allow_forking               = false
  allow_merge_commit          = true
  allow_rebase_merge          = true
  allow_squash_merge          = true
  allow_update_branch         = false
  delete_branch_on_merge      = false
  merge_commit_message        = "PR_TITLE"
  merge_commit_title          = "MERGE_MESSAGE"
  squash_merge_commit_message = "COMMIT_MESSAGES"
  squash_merge_commit_title   = "COMMIT_OR_PR_TITLE"
  web_commit_signoff_required = false
}
