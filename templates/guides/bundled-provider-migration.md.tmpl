---
page_title: "Migrating from the bundled GitHub Settings provider"
subcategory: ""
description: |-
  Transfer existing workflow permissions and switch the remaining resources to the independent provider.
---

# Migrating from the bundled GitHub Settings provider

Use this guide if you installed a bundled copy of GitHub Settings from an OpenTofu filesystem mirror and already have resources in state. Keep the old binary and its source address available until the transfer succeeds.

The independent provider has a different source address. It also drops `githubsettings_repository_workflow_permissions`. GitHub's default workflow permissions now belong to the [integrations/github resource](https://registry.terraform.io/providers/integrations/github/latest/docs/resources/workflow_repository_permissions). The example uses a single repository. Repeat the mapping for each address returned by `tofu state list`, including its module path where applicable.

## Transfer workflow permissions

1. Back up the state using your backend's usual protections. Keep the backup private and record the bundled provider's exact source from `tofu providers`. With the bundled provider still selected, run `tofu plan -detailed-exitcode` and require exit code 0. Retain the GitHub owner, credentials, and provider alias for each repository when selecting the `github` provider below.
2. Replace the old resource with these blocks. Preserve both permission values and use the repository name as the new import ID.

   ```hcl
   removed {
     from = githubsettings_repository_workflow_permissions.settings

     lifecycle {
       destroy = false
     }
   }

   resource "github_workflow_repository_permissions" "settings" {
     repository                       = "example-repository"
     default_workflow_permissions     = "read"
     can_approve_pull_request_reviews = false
   }

   import {
     to = github_workflow_repository_permissions.settings
     id = "example-repository"
   }
   ```

3. Save a plan using `tofu plan -out=workflow-permissions.tfplan`. Before applying it, run the following check. It requires at least one transfer, counts matching forget and import operations and rejects changes to remote resources. For each migration, substitute your exact addresses and repository import IDs for the example mapping.

   ```sh
   tofu show -json workflow-permissions.tfplan | jq -e '
     [.resource_changes[]? | select(.change.actions == ["forget"]) | .address]
       == ["githubsettings_repository_workflow_permissions.settings"]
     and
     [.resource_changes[]? | select(.change.importing) | [.address, .change.importing.id]]
       == [["github_workflow_repository_permissions.settings", "example-repository"]]
     and all(.resource_changes[]?; .change.actions == ["no-op"] or .change.actions == ["forget"])
   ' >/dev/null &&
   tofu apply workflow-permissions.tfplan &&
   tofu plan -detailed-exitcode
   ```

   The last plan must exit 0. Keep saved plans private and remove them when verification completes.

## Switch the remaining resources

1. Remove the completed migration blocks. In a separate working copy, replace the bundled source with `registry.terraform.io/elephant-evergreen/githubsettings` in the root and all child modules. Set a compatible version constraint in the root, such as `~> 0.0.2`. Run `tofu init -backend=false` in this copy to install the signed provider and generate its lock file.
2. Return to the initialized working copy that still uses the bundled provider and points to the existing backend. Run `tofu state replace-provider OLD_SOURCE registry.terraform.io/elephant-evergreen/githubsettings` and confirm the listed resources. Replace `OLD_SOURCE` with the full source recorded earlier. This changes provider addresses in state and leaves the resource addresses and GitHub settings intact. If no resources use the old source after the transfer, skip this step.
3. In the working copy with the new source, run `tofu init -reconfigure -lockfile=readonly` against the same backend, then run `tofu plan -detailed-exitcode`. Require exit code 0 before removing the old mirror or committing the new lock file.

If a verification plan returns changes, stop before applying it. Keep both provider installations available. If only the provider address changed, restore the old configuration and reverse `state replace-provider` before replanning. Restore a state backup only after confirming there were no intervening state writes. An already completed workflow-permissions transfer can stay on `integrations/github` while other settings remain on the bundled provider.
