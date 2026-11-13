# Terraform Provider for GitHub Settings

Manage selected GitHub repository and organization settings with Terraform. Install [elephant-evergreen/githubsettings from Terraform Registry](https://registry.terraform.io/providers/elephant-evergreen/githubsettings).

```hcl
terraform {
  required_providers {
    githubsettings = {
      source  = "elephant-evergreen/githubsettings"
      version = "0.0.1"
    }
  }
}

provider "githubsettings" {}
```

Set `GITHUB_TOKEN` to a token with access to the configured repositories or organization, then run `terraform init`, `terraform plan`, and `terraform apply`. The provider has no configuration arguments. It manages existing GitHub settings; destroying a resource removes it from Terraform state only and leaves the GitHub setting unchanged. Import repository settings with `OWNER/REPOSITORY` and organization settings with `OWNER`.

The provider manages 13 settings: three read-only resources that record GitHub settings and fail apply when configuration differs, and ten write resources that apply configuration to GitHub. See the [provider reference](docs/index.md) for arguments, attributes, permissions, and import syntax.

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
