# Terraform Provider for GitHub Settings

Manage selected GitHub repository and organization settings with Terraform. Install [elephant-evergreen/githubsettings from Terraform Registry](https://registry.terraform.io/providers/elephant-evergreen/githubsettings).

## Why?

Use [`integrations/github`](https://registry.terraform.io/providers/integrations/github/latest/docs) for repositories, teams, branch rulesets, and standard Actions settings. Add GitHub Settings for policies that provider does not expose, such as [approval for workflows from fork pull requests](docs/resources/repository_fork_contributor_approval.md), [immutable OIDC subjects for Actions](docs/resources/repository_oidc_subject_template.md), or [an organization's policy on adding deploy keys](docs/resources/organization_deploy_keys.md).

Use both providers in the same configuration and assign each setting to one provider. GitHub Settings manages policies on existing repositories and organizations. Import records the current setting in Terraform state, and removing a resource from management leaves the GitHub setting in place.

[Browse all supported settings](docs/resources/). For upgrades, read the [release notes](https://github.com/elephant-evergreen/terraform-provider-githubsettings/releases) or the [migration guide for bundled OpenTofu builds](docs/guides/bundled-provider-migration.md).

## Quickstart

Follow [Import a policy in Terraform Registry](https://registry.terraform.io/providers/elephant-evergreen/githubsettings/latest/docs#import-a-policy) to read a repository's fork pull request approval policy and record it in Terraform state.

## Development

Run `mise run check` for formatting, tests, vulnerability checks, builds, and generated documentation. Commit updated documentation when the generator changes `docs/`.

## Release

The release workflow builds, signs, and verifies packages when a version tag is pushed. It publishes the GitHub release and verifies that the signed version installs from Terraform Registry. See [RELEASE.md](RELEASE.md) for versioning, release notes, and signing.

## License

[MPL-2.0](LICENSE). This project is not affiliated with or endorsed by GitHub or HashiCorp.
