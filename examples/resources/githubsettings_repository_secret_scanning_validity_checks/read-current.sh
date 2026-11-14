GH_TOKEN="${GITHUB_TOKEN:?Set GITHUB_TOKEN first}" gh api --hostname github.com repos/OWNER/REPOSITORY \
  -H "X-GitHub-Api-Version: 2026-03-10" \
  --jq '.security_and_analysis.secret_scanning_validity_checks.status'
