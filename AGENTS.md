# Repository instructions

- Write public documentation, examples, diagnostics, issues, pull requests, and commit messages in English.
- Use Go and HashiCorp's official Terraform Plugin Framework. Keep the development toolchain in mise.toml and run `mise run check` before publication.
- Keep examples synthetic. Ordinary tests use local fixtures and must not call real GitHub repositories or organizations.
- Before designing a compatibility change, identify existing configurations, state, and installations, including bundled copies. Follow the [upgrade verification](RELEASE.md#verify-upgrades-before-merging) before merging.
- Do not add code comments unless explicitly requested. Preserve MPL-2.0 and third-party notices.
- Never commit credentials, Terraform state, plan files, or machine-specific paths.
- Include the reason for a change and a Co-Authored-By trailer identifying the AI tool and model in AI-assisted commits.
