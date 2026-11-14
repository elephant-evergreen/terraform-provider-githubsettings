GH_TOKEN="${GITHUB_TOKEN:?Set GITHUB_TOKEN first}" gh api --hostname github.com repos/OWNER/REPOSITORY \
  -H "X-GitHub-Api-Version: 2026-03-10" \
  --jq '{
  archived,
  allow_auto_merge,
  allow_forking,
  allow_merge_commit,
  allow_rebase_merge,
  allow_squash_merge,
  allow_update_branch,
  delete_branch_on_merge,
  merge_commit_message,
  merge_commit_title,
  squash_merge_commit_message,
  squash_merge_commit_title,
  web_commit_signoff_required
}'
