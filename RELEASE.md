# Release a provider version

The local release command signs packages, verifies uploaded files and tagged notes, and then publishes the GitHub release. The first release is `v0.0.1`.

For the first public release, use GitHub repository `elephant-evergreen/terraform-provider-githubsettings` to the `elephant-evergreen` namespace in the Terraform Registry. These steps follow HashiCorp's [provider publishing](https://developer.hashicorp.com/terraform/registry/providers/publishing) and [public namespace artifact](https://developer.hashicorp.com/terraform/cloud-docs/users-teams-organizations/organizations/public-namespace/artifacts) documentation.

## Decide when to release

Merge verified changes as they are ready. Publish when users need a new feature, a fix, or an important clarification. Release a compatible fix promptly when it blocks real use or addresses a security issue. Combine routine fixes, internal maintenance, and documentation improvements into the next useful release.

Before scheduling a breaking release, look for a compatible default, a deprecation period, or a state migration that preserves existing objects. Group related incompatible changes so users make one upgrade. Publish once the upgrade instructions and representative plans agree.

## Choose the version and write the notes

| Change | Version while on 0.x |
| --- | --- |
| First release | `v0.0.1` |
| Existing configurations or state require migration, or a documented operation changes incompatibly | Next minor, such as `v0.0.1` → `v0.1.0` |
| Compatible feature, fix, documentation, or dependency update | When released, next patch, such as `v0.0.1` → `v0.0.2` |
| Optional release candidate for explicit testing | Target version plus `-rc.1`, then `-rc.2` for a revised candidate |

Once the provider reaches 1.0, use [Semantic Versioning](https://semver.org/). Treat incompatible changes to import IDs, defaults, deletion behavior, and state formats as upgrade changes.

Add `release-notes/<tag>.md` in English to the release PR. Write for someone deciding whether to upgrade.

- Start with **Breaking changes** when existing users must act or can encounter incompatible behavior. For each change, identify the resource or setting, affected versions and configurations, what changed, and the action to take before applying. Link to a versioned upgrade guide for multi-step changes. Include a pre-upgrade plan, what to back up, the expected post-upgrade plan, and how to recover. State if an older provider cannot read upgraded state.
- Start with **Upgrade notes** for other releases. State whether existing users need to change HCL or state. For a first release, tell readers how to adopt existing objects or settings.
- Use **Highlights** and **Fixes** where applicable. Name the affected resource or setting and explain the behavior the user will see. Link to the relevant PR for context. Use **Compatibility** when supported platforms or verification change, and distinguish fixture results from live verification.
- End with a GitHub comparison from the previous public stable tag to this tag. For the first release, link to the versioned provider reference instead. Use absolute URLs and pin links to supporting documentation to the tag so they work in GitHub Releases.

A compatible fix can be brief. For example, lead with `## Upgrade notes` and “Existing configurations and state need no changes.” Follow with the affected resource and the outcome under `## Fixes`, then the tag comparison.

## One-time setup

Do these steps once before the first release.

1. Claim the `elephant-evergreen` public namespace in HCP Terraform, or confirm that the organization already owns it. Your HCP Terraform user needs the **Manage public providers** permission.

2. Install the Terraform Cloud GitHub App for the repository. In the GitHub organization settings, choose **Only select repositories** and select `terraform-provider-githubsettings`. HCP Terraform uses this installation to read the repository and its releases; it does not need access to every repository.

3. Create or select the release signing key. The Registry accepts RSA and DSA keys, but not the default ECC key type; use a separate RSA key for releases if your existing key uses ECC. If you do not have a release key, generate one in a temporary GnuPG home:

```sh
(
  set -eu
  umask 077
  GNUPGHOME="$(mktemp -d)"
  export GNUPGHOME
  trap 'gpgconf --kill gpg-agent 2>/dev/null || true; rm -rf -- "$GNUPGHOME"' EXIT
  trap 'exit 1' HUP INT TERM
  chmod 700 "$GNUPGHOME"
  gpg --quick-generate-key "terraform-provider-githubsettings release" rsa4096 cert,sign 2y
  gpg --armor --export-secret-keys > "$GNUPGHOME/release-private.asc"
  gpg --armor --export > "$GNUPGHOME/release-public.asc"
  gpg --fingerprint
  printf 'Save %s/release-private.asc and %s/release-public.asc in the password manager, then press Enter. ' "$GNUPGHOME" "$GNUPGHOME"
  read -r
)
```

Store `release-private.asc`, `release-public.asc`, the passphrase, and the full fingerprint in the password manager before you press Enter. The trap removes the temporary keyring when the block exits, including on failure or interruption, so the password-manager copy is the only retained copy. Never commit the private key, paste it into an issue or chat, or add it to GitHub Actions secrets.

4. Register only the public key in the Terraform Registry under **User Settings > Signing Keys**. Paste the contents of the saved `release-public.asc`; keep the private key out of the Registry.

5. Authenticate the GitHub CLI with an account that can create releases in the repository:

```sh
gh auth status --hostname github.com
```

If it reports no account, sign in:

```sh
gh auth login --hostname github.com --web
```

6. Install Bash, GitHub CLI, jq, and GnuPG, and confirm the pinned tools are available:

```sh
mise exec -- go version
mise exec -- terraform version
mise exec goreleaser@2.20.0 -- go version
mise exec goreleaser@2.20.0 -- goreleaser --version
```

The two Go version commands should report the same version from `mise.toml`. Check that the GoReleaser version is 2.20.0.

## Stage and verify a version

Set `TAG` to the reviewed release tag. Start from a clean checkout of the exact commit from `origin/main` and fetch the branch and tags. The tagged commit must contain `release-notes/$TAG.md`. For an incompatible release, run the documented migration with representative HCL and state and verify the resulting plan before tagging.

```sh
git fetch origin main --tags
mise run check
git tag -a "$TAG" -m "Release $TAG"
bin/release check-tag "$TAG"
```

Set `GPG_FINGERPRINT` to the full fingerprint of the registered key and `RELEASE_PUBLIC_KEY` to the path of its ASCII-armored public key. Export both. The corresponding private key must be available to GPG as `gpg --list-secret-keys "$GPG_FINGERPRINT"`. For unattended signing, use a release environment with the key already unlocked in its GPG agent. Keep the key and passphrase in the credential store.

```sh
mise exec goreleaser@2.20.0 -- goreleaser release --clean --skip=publish,announce
bin/release verify "$TAG"
```

A rehearsal ends here. To publish the verified packages, push the annotated tag and pass the repository explicitly:

```sh
git push origin "refs/tags/$TAG"
bin/release publish "$TAG" elephant-evergreen/terraform-provider-githubsettings
```

The command verifies the tag, notes, prerelease marker, detached signature, and uploaded SHA-256 digests before making the draft public. Retry an interrupted upload with the same command and the same tag. Use a new version when tagged files need a correction. Published tags and signed assets are immutable.

## Publish to the Registry

For the first release, publish the provider in HCP Terraform after the GitHub release is public:

1. Open the HCP Terraform organization, then **Registry > Public namespaces**.
2. Select `elephant-evergreen`, choose **Publish > Provider**, and select `terraform-provider-githubsettings`.
3. Choose a category, accept the terms, and click **Publish provider**.
4. Wait until the version list shows the selected tag as successfully processed. If processing fails, use the error shown in the namespace and release a new version after fixing it; keep the original signed assets fixed.

For later releases, the Registry's release webhook ingests the published GitHub release automatically. If a version is missing, use **Resync** on the provider page after confirming the Terraform Cloud GitHub App still has access to the repository.

## Verify the published provider

Once the provider is registered, verify the version with a direct, signed Registry install:

```sh
mise exec -- bin/release verify-registry "$TAG" elephant-evergreen/terraform-provider-githubsettings
```

The command waits for the exact version and installs it in an empty directory with the provider cache and CLI configuration disabled. After the first registration, run this command immediately after publishing. Registry prerequisites for first publication are listed above.
