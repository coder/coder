---
display_name: Docker (Repo Context Discovery)
description: Provision Docker containers as Coder workspaces with automatic AI agent context discovery on repository clone.
icon: ../../../site/static/icon/docker.png
maintainer_github: coder
verified: true
tags: [docker, container, ai, agents]
---

# Repository Context Discovery in Docker Workspaces

This template provisions a Docker-based Coder workspace, clones a Git repository during workspace startup, calls an external context service or a built-in mock, and writes generated agent context files that AI agent chats load automatically.

## Prerequisites

- A host with a working Docker socket available to the Coder deployment.
- A Coder deployment that supports the experimental agent context environment variables: `CODER_AGENT_EXP_INSTRUCTIONS_DIRS`, `CODER_AGENT_EXP_INSTRUCTIONS_FILE`, `CODER_AGENT_EXP_SKILLS_DIRS`, and `CODER_AGENT_EXP_SKILL_META_FILE`.

## Template Parameters

- `repo_url`: Required. The Git repository URL to clone into the workspace.
- `context_service_url`: Optional. The URL of an external service that returns generated context.
- `context_service_token`: Optional. A bearer token used when the external service requires authentication.

## Architecture

Startup clones follow this flow:

1. The workspace starts in a Docker container.
2. The `git-clone` module clones the repository into the workspace home directory.
3. The module runs `post_clone_script`.
4. `scripts/context-on-clone.sh` calls the external context service, or uses a built-in mock response when no service is configured.
5. The script writes generated files under `~/.coder/generated-context/<repo-name>/`.
6. The script attempts `coder chat context add --dir <per-repo-dir>` so an active chat can receive the new context immediately.

Runtime clones use the same script:

1. The workspace `startup_script` configures a Git template directory with a `post-checkout` hook through `init.templateDir`.
2. You run `git clone` from a terminal or chat session.
3. Git fires the hook on the initial checkout only, when the old commit is the zero SHA.
4. The hook calls `scripts/context-on-clone.sh`.
5. The script writes generated files under `~/.coder/generated-context/<repo-name>/`.
6. The script attempts `coder chat context add --dir <per-repo-dir>` again.

Generated files are written per repository:

```text
~/.coder/generated-context/<repo-name>/AGENTS.md
~/.coder/generated-context/<repo-name>/.agents/skills/<skill>/SKILL.md
```

Multiple repositories can coexist under `~/.coder/generated-context/`.

## External Context Service

Point `context_service_url` at a service that accepts a JSON POST body with repository and workspace details and returns JSON shaped like this:

```json
{
  "instructions": "# Repo instructions\n\nUse repository specific guidance here.",
  "skills": [
    {
      "name": "repo-overview",
      "description": "Provide a repository overview.",
      "body": "---\nname: repo-overview\ndescription: Provide a repository overview.\n---\n\n# Repo Overview\n\nSummarize the project layout and conventions."
    }
  ]
}
```

If `context_service_url` is empty, or the request fails, the template uses a built-in mock response so you can test the flow locally.

## Limitations

- Generated instructions and skills are advisory content and should be treated according to your own trust model.
- Existing chats do not live reload context files after startup. When the clone process can resolve an active chat, the script also runs `coder chat context add` to inject the new repository context into that chat.
- Automatic chat injection only works when the clone runs in a process that can resolve an active chat. Manual terminal clones still write files, but may not auto-inject. You can run `coder chat context add --chat <id> --dir <path>` manually.
- If multiple active chats exist, you may need the `--chat` flag to choose which chat receives the context.
- If the startup clone also triggers the `post-checkout` hook, the duplicate execution is idempotent.

This example is intended as a starting point for template-driven repository context discovery. Extend the Terraform and script to match your own service contract and security requirements.
