# Release a provider version

Releases are staged and signed locally. GitHub Releases presents the reviewed notes and signed packages for each tag. The first release is `v0.0.1`.

The provider is published from the public GitHub repository `elephant-evergreen/terraform-provider-githubsettings` to the `elephant-evergreen` namespace in the Terraform Registry. These steps follow HashiCorp's [provider publishing](https://developer.hashicorp.com/terraform/registry/providers/publishing) and [public namespace artifact](https://developer.hashicorp.com/terraform/cloud-docs/users-teams-organizations/organizations/public-namespace/artifacts) documentation.

## Choose the version and write the notes

| Change | Version while on 0.x |
| --- | --- |
| First release | `v0.0.1` |
| Existing configurations or state require migration, or a documented operation changes incompatibly | Next minor, such as `v0.0.1` → `v0.1.0` |
| Compatible feature, fix, documentation, or dependency update | Next patch, such as `v0.0.1` → `v0.0.2` |
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

6. Confirm the pinned tools are available:

```sh
mise exec -- go version
mise exec -- terraform version
mise exec goreleaser@2.20.0 -- go version
mise exec goreleaser@2.20.0 -- goreleaser --version
```

The two Go version commands should report the same version from `mise.toml`. Check that the GoReleaser version is 2.20.0.

## Stage and verify a version

Start from a clean, reviewed commit on the default branch:

```sh
git switch main
git pull --ff-only
git status --short
git log -1 --oneline
```

`git status --short` must print nothing. Run the repository checks and confirm that the generated documentation is already committed:

```sh
mise run check
mise exec -- go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...
mise exec -- go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs@v0.25.0 generate --provider-name githubsettings
git diff --exit-code -- docs examples
```

For an incompatible release, exercise the documented upgrade from the previous published version with representative HCL and state. Check that the plan contains only the intended actions. Set `VERSION` to the selected version and tag the checked commit locally. Keep the tag local until the checksums and signature pass:

```sh
VERSION=0.0.1
TAG="v${VERSION}"
PROVIDER=terraform-provider-githubsettings
NOTES="release-notes/${TAG}.md"
printf '%s\n' "$TAG" | grep -Eq '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-rc\.[1-9][0-9]*)?$' &&
  awk 'NF { print; exit }' "$NOTES" | grep -Eq '^## (Upgrade notes|Breaking changes)$' &&
  test "$(awk 'NF { count++ } END { print count+0 }' "$NOTES")" -ge 2 &&
  git tag -a "$TAG" -m "Release $TAG"
```

Build the archives, checksums, and release metadata without uploading anything. `mise exec` supplies both the pinned GoReleaser and the repository Go toolchain:

```sh
mise exec goreleaser@2.20.0 -- goreleaser release --clean --skip=publish,sign --release-notes="$NOTES"
```

Run the remaining commands in the same shell so `VERSION`, `TAG`, `PROVIDER`, and `NOTES` stay set.

`--skip=publish` leaves the release on the local machine. `--skip=sign` leaves signing to the explicit GPG command below, so the passphrase path stays interactive.

GoReleaser writes the manifest checksum but does not copy the manifest into `dist/` when publishing is skipped. Copy it with the release asset name and verify every checksum, including the manifest:

```sh
cp terraform-registry-manifest.json "dist/${PROVIDER}_${VERSION}_manifest.json"
cd dist
shasum -a 256 -c "${PROVIDER}_${VERSION}_SHA256SUMS"
cd ..
```

Set `GPG_FINGERPRINT` to the full fingerprint of the signing key registered with the Registry. Import the key into a temporary GnuPG home in a subshell:

```sh
(
  set -eu
  umask 077
  GNUPGHOME="$(mktemp -d)"
  export GNUPGHOME
  trap 'gpgconf --kill gpg-agent 2>/dev/null || true; rm -rf -- "$GNUPGHOME"' EXIT
  trap 'exit 1' HUP INT TERM
  chmod 700 "$GNUPGHOME"

  printf 'Save the ASCII-armored private key from the password manager as %s/release-private.asc, then press Enter. ' "$GNUPGHOME"
  read -r
  test -s "$GNUPGHOME/release-private.asc"

  gpg --import "$GNUPGHOME/release-private.asc"
  rm -f "$GNUPGHOME/release-private.asc"

  test -n "${GPG_FINGERPRINT:-}"
  imported_fingerprint="$(gpg --with-colons --list-secret-keys "$GPG_FINGERPRINT" | awk -F: '$1 == "fpr" { print $10; exit }')"
  test "$imported_fingerprint" = "$GPG_FINGERPRINT"

  gpg --local-user "$GPG_FINGERPRINT" \
    --output "dist/${PROVIDER}_${VERSION}_SHA256SUMS.sig" \
    --detach-sign "dist/${PROVIDER}_${VERSION}_SHA256SUMS"
  gpg --verify "dist/${PROVIDER}_${VERSION}_SHA256SUMS.sig" "dist/${PROVIDER}_${VERSION}_SHA256SUMS"
)
```

The script checks the imported key against the registered fingerprint before signing. `gpg --verify` must report a good signature. The Registry requires a binary `.sig` file. Enter the passphrase through GPG's normal pinentry.

Check that every archive contains the versioned provider binary:

```sh
for platform in darwin_amd64 darwin_arm64 linux_amd64 linux_arm64; do
  unzip -l "dist/${PROVIDER}_${VERSION}_${platform}.zip"
done
```

Each listing must include `terraform-provider-githubsettings_v${VERSION}` with the selected version. The four archives are `darwin_amd64`, `darwin_arm64`, `linux_amd64`, and `linux_arm64`.

If a tagged file needs a correction while the tag is still local, delete the local tag with `git tag -d "$TAG"`, review the correction, and tag the corrected commit. For a release rehearsal, stop after verifying the archives. To publish, push the tag after all checks pass:

```sh
git push origin "refs/tags/$TAG"
```

## Create the draft GitHub release

Create a draft release and upload the staged assets. The tag is now on GitHub, so `--verify-tag` prevents the CLI from creating a different tag:

```sh
prerelease=false
case "$VERSION" in
  *-rc.*) prerelease=true ;;
esac
gh release create "$TAG" --verify-tag --draft --prerelease="$prerelease" --title "$TAG" --notes-file "$NOTES" \
  "dist/${PROVIDER}_${VERSION}_darwin_amd64.zip" \
  "dist/${PROVIDER}_${VERSION}_darwin_arm64.zip" \
  "dist/${PROVIDER}_${VERSION}_linux_amd64.zip" \
  "dist/${PROVIDER}_${VERSION}_linux_arm64.zip" \
  "dist/${PROVIDER}_${VERSION}_manifest.json" \
  "dist/${PROVIDER}_${VERSION}_SHA256SUMS" \
  "dist/${PROVIDER}_${VERSION}_SHA256SUMS.sig"
```

Inspect the draft before making it public:

```sh
gh release view "$TAG" --json isDraft,isPrerelease,body,assets \
  --jq '{draft: .isDraft, prerelease: .isPrerelease, notes: .body, assets: [.assets[].name]}'
```

Confirm that the draft contains the reviewed notes and the seven expected assets. For an RC tag, confirm that the draft is marked as a prerelease. Retry transient upload failures against the same unpublished draft after checking which assets are present. If files from a pushed tag need a correction, use a new version. Keep published tags and signed assets fixed. Publish factual corrections to the notes in the release body with a dated correction.

Publish the draft:

```sh
gh release edit "$TAG" --draft=false
```

## Publish to the Registry

For the first release, publish the provider in HCP Terraform after the GitHub release is public:

1. Open the HCP Terraform organization, then **Registry > Public namespaces**.
2. Select `elephant-evergreen`, choose **Publish > Provider**, and select `terraform-provider-githubsettings`.
3. Choose a category, accept the terms, and click **Publish provider**.
4. Wait until the version list shows the selected tag as successfully processed. If processing fails, use the error shown in the namespace and release a new version after fixing it; keep the original signed assets fixed.

For later releases, the Registry's release webhook ingests the published GitHub release automatically. If a version is missing, use **Resync** on the provider page after confirming the Terraform Cloud GitHub App still has access to the repository.

## Verify the published provider

Run a direct Registry init in a fresh directory. The subshell disables CLI configuration and the provider cache, and it resolves the Terraform binary before changing directory:

```sh
(
  set -eu
  workdir="$(mktemp -d)"
  terraform_bin="$(mise which terraform 2>/dev/null || command -v terraform)"
  trap 'rm -rf -- "$workdir"' EXIT
  trap 'exit 1' HUP INT TERM
  cd "$workdir"
  unset TF_PLUGIN_CACHE_DIR TF_DATA_DIR
  cat > main.tf <<EOF
terraform {
  required_providers {
    githubsettings = {
      source  = "elephant-evergreen/githubsettings"
      version = "= $VERSION"
    }
  }
}
EOF
  if ! output="$(TF_CLI_CONFIG_FILE=/dev/null "$terraform_bin" init -no-color 2>&1)"; then
    printf '%s\n' "$output" >&2
    exit 1
  fi
  printf '%s\n' "$output"
  printf '%s\n' "$output" | grep -F "Installed elephant-evergreen/githubsettings v$VERSION (" | grep -Eq '\((self-signed, key ID |signed by )'
  TF_CLI_CONFIG_FILE=/dev/null "$terraform_bin" providers
)
```

For this namespace, the signed install line is normally `(self-signed, key ID ...)`, which means Terraform verified the checksum signature with the key registered in the Registry. This assertion applies only to the direct Registry init shown here; a local filesystem mirror or provider cache can install without the signature line.

Open the provider page in the Terraform Registry and confirm that the selected version is listed and its documentation renders. The subshell removes the temporary directory when it exits. A release candidate requires an exact pin. A higher-version candidate may appear as the newest version on the Registry page.
