# Mux Dogfood Template

A stripped-down, Mux-focused workspace template for developing
[Mux](https://github.com/coder/mux) using Mux on Coder.

This is a simplified version of the main `dogfood/coder` template. It
removes IDE choices, devcontainers, Claude Code task runner, and
Docker-in-Docker support in favor of a lean Mux-only experience.

## What's included

- **Mux** as the sole IDE (browser-based)
- **Auto-clone** of `https://github.com/coder/mux` into `~/mux`
- **AI Bridge** support for Anthropic API proxying
- **GitHub auth** with GH CLI login and Mux GitHub owner config
- Standard modules: dotfiles, git-config, personalize, coder-login

## What's excluded (compared to `dogfood/coder`)

- No IDE choices (VS Code, JetBrains, Cursor, Windsurf, Zed)
- No devcontainers or Docker-in-Docker (`sysbox-runc`)
- No Claude Code task runner or AI task support
- No workspace prebuilds
- No `large-5mb-module` stress test
- No boundary config or `develop.sh` scripts

## Deploying

This template is not wired into CI. Deploy manually:

```bash
cd dogfood/mux
terraform init
terraform validate

# Push to dev.coder.com (requires CODER_URL and CODER_SESSION_TOKEN)
coder templates push mux-dogfood --directory .
```

## Image

Uses `codercom/oss-dogfood:latest` — the same image built by the main
dogfood CI pipeline. No custom Dockerfile needed.
