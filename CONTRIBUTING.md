# Contributing

## Public information

This repository, its releases, and its issue tracker are public. Do not include private repository names, internal project names, credentials, real organization or repository configuration, tokens, Terraform state, saved plans, or per-device update and verification history in public documentation, examples, diagnostics, issues, pull requests (including descriptions and comments), commit messages, or release notes.

Use synthetic values in examples and fixtures. For tests and documentation, use synthetic GitHub organizations, repositories, and tokens. Do not include real organization or repository configuration, tokens, state, or saved plans.

Document supported and tested versions separately from any per-device history. State supported and tested versions in release notes or documentation, and keep per-device update and verification records out of public material.

## Checks

Run `mise run check` before opening a pull request. It covers Go formatting, module verification, vet, vulnerability checks, tests, builds, Terraform formatting, and generated documentation.
