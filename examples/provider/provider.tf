terraform {
  required_providers {
    githubsettings = {
      source  = "elephant-evergreen/githubsettings"
      version = "~> 0.0.4"
    }
  }
}

provider "githubsettings" {}

resource "githubsettings_repository_fork_contributor_approval" "example" {
  owner           = "OWNER"
  repository      = "REPOSITORY"
  approval_policy = "first_time_contributors"
}

import {
  to = githubsettings_repository_fork_contributor_approval.example
  id = "OWNER/REPOSITORY"
}
