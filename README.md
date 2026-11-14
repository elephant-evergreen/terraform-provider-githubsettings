# Terraform Provider for GitHub Settings

Manage selected GitHub repository and organization settings with Terraform. Install [elephant-evergreen/githubsettings from Terraform Registry](https://registry.terraform.io/providers/elephant-evergreen/githubsettings).

## Why?

Use [`integrations/github`](https://registry.terraform.io/providers/integrations/github/latest/docs) for repositories, teams, branch rulesets, and standard Actions settings. Add GitHub Settings for policies that provider does not expose, such as [approval for workflows from fork pull requests](docs/resources/repository_fork_contributor_approval.md), [immutable OIDC subjects for Actions](docs/resources/repository_oidc_subject_template.md), or [an organization's policy on adding deploy keys](docs/resources/organization_deploy_keys.md).

Use both providers in the same configuration and assign each setting to one provider. GitHub Settings manages policies on existing repositories and organizations. Import records the current setting in Terraform state, and removing a resource from management leaves the GitHub setting in place.

[Browse all supported settings](docs/resources/). For upgrades, read the [release notes](https://github.com/elephant-evergreen/terraform-provider-githubsettings/releases) or the [migration guide for bundled OpenTofu builds](docs/guides/bundled-provider-migration.md).

## Quickstart

Import one repository's fork pull request contributor approval policy into Terraform state. The import leaves the GitHub policy in place. Run it in a new working directory, or add the configuration to an existing Terraform project where appropriate. The provider reads `GITHUB_TOKEN` and needs no provider arguments. You need Terraform with [import block support](https://developer.hashicorp.com/terraform/language/block/import) and the [GitHub CLI](https://cli.github.com/) (`gh`) installed.

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
      version = "~> 0.0.2"
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

5. Add the import block to `main.tf` using the same `OWNER/REPOSITORY`:

```terraform
import {
  to = githubsettings_repository_fork_contributor_approval.example
  id = "OWNER/REPOSITORY"
}
```

6. Install the provider with `terraform init`.
7. Run `terraform plan -out=adopt.tfplan`. Expect `1 to import, 0 to add, 0 to change, 0 to destroy`. If it includes a policy change, compare the configuration with the output from step 3.
8. Run `terraform apply adopt.tfplan` to record the policy in Terraform state.
9. Run `terraform plan` again and expect `No changes`.
10. Remove the token from your shell:

```sh
unset GITHUB_TOKEN
```

## Development

Run `mise run check` for formatting, tests, vulnerability checks, builds, and generated documentation. Commit updated documentation when the generator changes `docs/`.

## Release

The release workflow builds, signs, and verifies packages when a version tag is pushed. It publishes the GitHub release and verifies that the signed version installs from Terraform Registry. See [RELEASE.md](RELEASE.md) for versioning, release notes, and signing.

## License

[MPL-2.0](LICENSE). This project is not affiliated with or endorsed by GitHub or HashiCorp.
