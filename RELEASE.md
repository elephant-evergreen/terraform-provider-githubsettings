# Release a provider version

Releases are manual. There is no GitHub Actions workflow and no unsigned fallback. The first release is `v0.0.1`; use the same steps for later versions after replacing `0.0.1` with the new version.

The provider is published from the public GitHub repository `elephant-evergreen/terraform-provider-githubsettings` to the `elephant-evergreen` namespace in the Terraform Registry. These steps follow HashiCorp's [provider publishing](https://developer.hashicorp.com/terraform/registry/providers/publishing) and [public namespace artifact](https://developer.hashicorp.com/terraform/cloud-docs/users-teams-organizations/organizations/public-namespace/artifacts) documentation.

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

## Stage and verify v0.0.1

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
mise exec -- go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs@v0.25.0 generate --provider-name githubsettings
git diff --exit-code -- docs examples
```

Do not commit during the release. After the checks pass, create the release tag locally. The tag must be a valid semantic version prefixed with `v`, and no branch may use the same name. Keep the tag local until the build and signature verification below pass:

```sh
git tag -a v0.0.1 -m "v0.0.1"
```

Build the archives, checksums, and release metadata without uploading anything. `mise exec` supplies both the pinned GoReleaser and the repository Go toolchain:

```sh
VERSION=0.0.1
TAG="v${VERSION}"
PROVIDER=terraform-provider-githubsettings
mise exec goreleaser@2.20.0 -- go version
mise exec goreleaser@2.20.0 -- goreleaser release --clean --skip=publish --skip=sign
```

Run the remaining commands in the same shell so `VERSION`, `TAG`, and `PROVIDER` stay set.

`--skip=publish` leaves the release on the local machine. `--skip=sign` leaves signing to the explicit GPG command below, so the passphrase path stays interactive.

GoReleaser writes the manifest checksum but does not copy the manifest into `dist/` when publishing is skipped. Copy it with the release asset name and verify every checksum, including the manifest:

```sh
cp terraform-registry-manifest.json "dist/${PROVIDER}_${VERSION}_manifest.json"
cd dist
shasum -a 256 -c "${PROVIDER}_${VERSION}_SHA256SUMS"
cd ..
```

Import the release key into a temporary GnuPG home in a subshell. The trap removes only that home on any exit; it does not touch the password-manager copy:

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

  GPG_FINGERPRINT="$(gpg --with-colons --list-secret-keys | awk -F: '$1 == "fpr" { print $10; exit }')"
  test -n "$GPG_FINGERPRINT"
  gpg --fingerprint "$GPG_FINGERPRINT"

  gpg --local-user "$GPG_FINGERPRINT" \
    --output "dist/${PROVIDER}_${VERSION}_SHA256SUMS.sig" \
    --detach-sign "dist/${PROVIDER}_${VERSION}_SHA256SUMS"
  gpg --verify "dist/${PROVIDER}_${VERSION}_SHA256SUMS.sig" "dist/${PROVIDER}_${VERSION}_SHA256SUMS"
)
```

`gpg --with-colons --list-secret-keys` prints the full fingerprint in the `fpr` field, and `gpg --fingerprint` displays it for confirmation. The fingerprint must match the key registered in step 4. `gpg --verify` must report a good signature. The signature must be binary; do not pass `--armor`, because the Registry requires a binary `.sig` file. Signing prompts for the key passphrase through GPG's normal pinentry; do not put the passphrase on the command line or in a file in the repository.

Check that every archive contains the versioned provider binary:

```sh
for platform in darwin_amd64 darwin_arm64 linux_amd64 linux_arm64; do
  unzip -l "dist/${PROVIDER}_${VERSION}_${platform}.zip"
done
```

Each listing must include `terraform-provider-githubsettings_v0.0.1`. The four archives are `darwin_amd64`, `darwin_arm64`, `linux_amd64`, and `linux_arm64`.

If any check fails, delete the local tag with `git tag -d v0.0.1`, fix the cause, and repeat from the clean commit. Push the tag only after the checksums, manifest, signature, and all four archives pass:

```sh
git push origin v0.0.1
```

## Create the draft GitHub release

Create a draft release and upload the staged assets. The tag is now on GitHub, so `--verify-tag` prevents the CLI from creating a different tag:

```sh
gh release create "$TAG" --verify-tag --draft --title "$TAG" --notes "Initial release." \
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
gh release view "$TAG" --json isDraft,assets \
  --jq '{draft: .isDraft, assets: [.assets[].name]}'
```

Confirm that `draft` is `true` and that all seven assets are present: four archives, the manifest, the checksums, and the checksums signature. If an asset is wrong, do not publish the draft; fix the cause and rebuild before publishing. Do not edit or replace an already-published release.

Publish the draft:

```sh
gh release edit "$TAG" --draft=false
```

## Publish to the Registry

For the first release, publish the provider in HCP Terraform after the GitHub release is public:

1. Open the HCP Terraform organization, then **Registry > Public namespaces**.
2. Select `elephant-evergreen`, choose **Publish > Provider**, and select `terraform-provider-githubsettings`.
3. Choose a category, accept the terms, and click **Publish provider**.
4. Wait until the version list shows `v0.0.1` as successfully processed. If processing fails, use the error shown in the namespace and release a new version after fixing it; do not replace the `v0.0.1` assets.

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
  cat > main.tf <<'EOF'
terraform {
  required_providers {
    githubsettings = {
      source  = "elephant-evergreen/githubsettings"
      version = "0.0.1"
    }
  }
}
EOF
  if ! output="$(TF_CLI_CONFIG_FILE=/dev/null "$terraform_bin" init -no-color 2>&1)"; then
    printf '%s\n' "$output" >&2
    exit 1
  fi
  printf '%s\n' "$output"
  printf '%s\n' "$output" | grep -E 'Installed elephant-evergreen/githubsettings v0\.0\.1 \(.*signed'
  TF_CLI_CONFIG_FILE=/dev/null "$terraform_bin" providers
)
```

The install line must include `signed`; for this namespace it is normally `(self-signed, key ID ...)`, which means Terraform verified the checksum signature with the key registered in the Registry. This assertion applies only to the direct Registry init shown here; a local filesystem mirror or provider cache can install without the signature line.

Open the provider page in the Terraform Registry and confirm that `v0.0.1` is listed and its documentation renders. The subshell removes the temporary directory when it exits.
