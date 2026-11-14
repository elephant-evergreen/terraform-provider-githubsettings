# Terraform Provider for GitHub Settings

Manage selected GitHub repository and organization settings with Terraform. Install [elephant-evergreen/githubsettings from Terraform Registry](https://registry.terraform.io/providers/elephant-evergreen/githubsettings).

The provider has no configuration arguments. Terraform reads a GitHub API token from the `GITHUB_TOKEN` environment variable. It manages existing settings; it does not create repositories or organizations. Destroying a resource removes it from Terraform state without changing the GitHub setting.

## Quickstart

This inspects one repository's fork pull request contributor approval policy, imports it, and plans without changing GitHub. Run it in a new working directory, or add the configuration to an existing Terraform project where appropriate. You need Terraform and the [GitHub CLI](https://cli.github.com/) (`gh`) installed.

1. Check your `gh` login for github.com:

```sh
gh auth status --hostname github.com
```

If it reports no account, sign in:

```sh
gh auth login --hostname github.com --web
```

2. Export a token for Terraform. The command substitution keeps the token out of terminal output:

```sh
GITHUB_TOKEN="$(gh auth token --hostname github.com)" && export GITHUB_TOKEN
```

`GITHUB_TOKEN` supplies local GitHub API credentials to the provider.

For this setting, a fine-grained personal access token needs repository `Administration: Read` to inspect and `Administration: Read and write` to update. OAuth apps and classic PATs need the `repo` scope. The interactive `gh` login above uses an OAuth app token, which needs the `repo` scope; `gh auth status` shows the token scopes. If the `gh` token has access to the repository, you do not need a separate personal access token. Other settings require different permissions; see the [resource documentation](docs/resources/) for each resource.

3. Inspect the current policy. Replace `OWNER` and `REPOSITORY` with the account and repository to manage:

```sh
gh api repos/OWNER/REPOSITORY/actions/permissions/fork-pr-contributor-approval \
  --jq '{approval_policy}'
```

The output shows the live value, for example:

```json
{
  "approval_policy": "first_time_contributors"
}
```

4. Create `main.tf` in the working directory. Replace `OWNER`, `REPOSITORY`, and the example `approval_policy` with the values from step 3:

```terraform
terraform {
  required_providers {
    githubsettings = {
      source  = "elephant-evergreen/githubsettings"
      version = "0.0.1"
    }
  }
}

provider "githubsettings" {}

resource "githubsettings_repository_fork_contributor_approval" "example" {
  owner           = "OWNER"
  repository      = "REPOSITORY"
  approval_policy = "first_time_contributors"
}
```

This example pins the first release, v0.0.1.

5. Import the existing setting with the same `OWNER/REPOSITORY` and plan:

```sh
terraform init
terraform import githubsettings_repository_fork_contributor_approval.example OWNER/REPOSITORY
terraform plan
```

Import records the current GitHub setting in Terraform state without changing it. When the configuration matches the live value, `terraform plan` reports `No changes`. If it reports a difference, compare it with the output from step 3.

6. Remove the token from your shell:

```sh
unset GITHUB_TOKEN
```

## Managed settings

The provider manages 13 settings: three read-only resources that record GitHub settings and fail apply when configuration differs, and ten write resources that apply configuration to GitHub. See the [resource documentation](docs/resources/) for arguments, attributes, permissions, and import syntax.

## Development

Run `mise run check` to check Go formatting, verify modules, run vet and tests, and check Terraform formatting in `examples/`. Documentation is generated with the standard `tfplugindocs`:

```sh
go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs@v0.25.0 generate --provider-name githubsettings
```

## Release

Releases are manual. From a reviewed commit on the default branch:

```sh
git tag v0.0.1
GPG_FINGERPRINT=<key> goreleaser release --clean
```

GoReleaser builds the Linux and macOS archives, SHA-256 checksums, Registry manifest, and detached signature. `GITHUB_TOKEN` and the signing key are supplied by the operator's environment and are never committed.

## License

[MPL-2.0](LICENSE). This project is not affiliated with or endorsed by GitHub or HashiCorp.
