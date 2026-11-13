terraform {
  required_providers {
    githubsettings = {
      source = "elephant-evergreen/githubsettings"
    }
  }
}

provider "githubsettings" {}
