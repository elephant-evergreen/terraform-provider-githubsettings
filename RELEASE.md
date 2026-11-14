# Release a provider version

Users find changes and upgrade instructions in [GitHub Releases](https://github.com/elephant-evergreen/terraform-provider-githubsettings/releases). The tag-triggered Release workflow verifies the signed packages and tagged notes before publishing to GitHub, then installs the exact version from [Terraform Registry](https://registry.terraform.io/providers/elephant-evergreen/githubsettings).

## Decide when to release

Merge verified changes as they are ready. Publish when users need a new feature, a fix, or an important clarification. Release a compatible fix promptly when it blocks real use or addresses a security issue. Combine routine fixes, internal maintenance, and documentation improvements into the next useful release.

Before scheduling a breaking release, look for a compatible default, a deprecation period, or a state migration that preserves existing objects. Group related incompatible changes so users make one upgrade. Publish once the upgrade instructions and representative plans agree.

## Verify upgrades before merging

Before merging an incompatible change, exercise the documented upgrade from each affected installation path with representative configuration and state in a test environment. Include the [bundled provider migration](docs/guides/bundled-provider-migration.md) when the change affects bundled installations. Confirm that the plan contains only the documented actions and that a subsequent plan preserves the existing objects and settings. Reuse the results for the same release candidate and repeat any checks affected by later changes.

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

## Publish and verify

1. Review the notes with the changes they describe. For an incompatible release, confirm that the [upgrade verification](#verify-upgrades-before-merging) covers the candidate.
2. After the PR is merged and Check passes on the exact main-branch commit, set `release_commit` to that SHA and `release_tag` to the version matching the notes. Create and push an annotated tag.

   ```sh
   git fetch origin main --tags &&
   git tag -a "$release_tag" "$release_commit" -m "Release $release_tag" &&
   git push origin "refs/tags/$release_tag"
   ```

3. The **Release** workflow checks the tagged commit, builds and signs the packages, and creates or resumes the GitHub draft. It verifies the notes and uploaded digests before publishing. Retry failed uploads on the same tag. Use a new version if tagged files need a correction. Keep published tags and signed assets unchanged.
4. The workflow waits for the Registry to serve the exact version and verifies a signed installation from an empty directory. If ingestion times out, confirm that the Terraform Cloud GitHub App still has access, inspect the version in HCP Terraform, and use **Resync** if its preview would only add the intended version. Then rerun the workflow on the same tag.

## Registry and signing setup

The provider is registered under the `elephant-evergreen` public namespace in HCP Terraform. Give the Terraform Cloud GitHub App access to this repository through **Only select repositories**. See [HashiCorp's public namespace guide](https://developer.hashicorp.com/terraform/cloud-docs/users-teams-organizations/organizations/public-namespace/artifacts).

The [release public key](keys/release.asc) is registered in that namespace. Store its matching private signing key in the repository's `GPG_PRIVATE_KEY` Actions secret and keep the source copy in the credential store. When rotating the key, register the new public key alongside the existing one, update the committed public key and Actions secret, and retain the old public key to verify past releases. See [HashiCorp's provider publishing guide](https://developer.hashicorp.com/terraform/registry/providers/publishing).
