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

3. Save a plan using `tofu plan -out=workflow-permissions.tfplan`. The plan must forget the old addresses and import the corresponding new addresses with zero creates, updates, replacements, or deletes. Apply that saved plan and require a subsequent `tofu plan -detailed-exitcode` to exit 0.

## Switch the remaining resources

1. Remove the completed migration blocks. In the root and all child modules, replace the bundled source with `registry.terraform.io/elephant-evergreen/githubsettings`. In the root, use a compatible version constraint such as `~> 0.0.1`. Run `tofu init -upgrade` to install the independent provider and update the lockfile.
2. Run `tofu state replace-provider -auto-approve OLD_SOURCE registry.terraform.io/elephant-evergreen/githubsettings`. Replace `OLD_SOURCE` with the full bundled source recorded above. This changes the provider address in state and leaves the resource addresses and GitHub settings intact. If no resources use the old source after the transfer, skip this command.
3. Run `tofu plan -detailed-exitcode`. Require exit code 0 before removing the old mirror or committing the new lockfile.

If a verification plan returns changes, stop before applying it. Keep both provider installations available. If only the provider address changed, restore the old configuration and reverse `state replace-provider` before replanning. Restore a state backup only after confirming there were no intervening state writes. An already completed workflow-permissions transfer can stay on `integrations/github` while other settings remain on the bundled provider.
