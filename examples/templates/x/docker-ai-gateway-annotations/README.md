---
display_name: Docker + AI Gateway Annotations
description: Docker template that routes Claude Code through the AI Gateway and registers the interception annotation MCP tool
icon: ../../../../site/static/icon/docker.png
maintainer_github: coder
tags: [docker, container, ai-gateway, claude-code]
---

> **Experimental**: This template configures the experimental MCP HTTP
> endpoint and the interception annotation tool. Both are prototypes
> subject to change. Do not rely on this for production workloads.

# Docker + AI Gateway Annotations

This template provisions a Docker workspace where Claude Code is wired to
the Coder AI Gateway and can annotate its own recorded activity with the
work it is doing.

Two pieces of configuration make that work:

| Configuration                                 | Effect                                                                        |
|-----------------------------------------------|-------------------------------------------------------------------------------|
| `ANTHROPIC_BASE_URL` and `ANTHROPIC_AUTH_TOKEN` | Claude Code sends Anthropic traffic through the AI Gateway as the workspace owner, so each request is recorded as an interception. |
| MCP server `coder-ai-gateway`                   | Registers `coder_annotate_interception` with Claude Code so it can attach a repository, branch, Linear issues, and GitHub pull requests to the interception. |

The gateway serves each provider under its configured name, so
`ANTHROPIC_BASE_URL` is `<access-url>/api/v2/ai-gateway/<provider-name>`. Set
`ai_gateway_provider` to the provider name on the deployment; a request to a
name the gateway does not serve returns 404, which Claude Code reports as a
problem with the selected model.

The MCP server is registered against the `annotations` toolset:

```text
<access-url>/api/experimental/mcp/http?toolset=annotations
```

That toolset exposes only the annotation tool and sends server instructions
describing when to call it, so no prompt file or `CLAUDE.md` is needed.

## Deployment requirements

- A Premium license with AI Governance. `/api/v2/ai-gateway/*` requires the
  AI Bridge entitlement, and no interceptions are recorded without it.
- At least one Anthropic AI Gateway provider configured on the deployment,
  with its name set in `ai_gateway_provider`.
- The `oauth2` and `mcp-server-http` experiments enabled. Development
  builds bypass this check.
- An access URL the workspace container can reach. The template rewrites
  `localhost` and `127.0.0.1` to `host.docker.internal`; set
  `access_url_override` when that is not enough.

## Variables

| Variable              | Default     | Purpose                                          |
|-----------------------|-------------|--------------------------------------------------|
| `docker_socket`       | `""`        | Docker socket URI.                               |
| `claude_code_version` | `latest`    | Version passed to the Claude Code installer.     |
| `ai_gateway_provider` | `anthropic` | AI Gateway provider name serving Anthropic.      |
| `claude_model`        | `""`        | Pins `ANTHROPIC_MODEL` when set.                 |
| `access_url_override` | `""`        | Deployment URL to use inside the container.      |

## Verifying

1. Open the **Claude Code** app in the workspace and confirm the MCP server
   is connected with `/mcp`.
2. Ask Claude to work on something in a repository, or tell it the issue
   you are working on.
3. Open **AI Gateway** in the dashboard, select the session, and check the
   summary rows for repository, branch, Linear issues, and pull requests.
4. Export spend with the annotation columns:

   ```sh
   curl -H "Coder-Session-Token: $TOKEN" \
     "$CODER_URL/api/v2/organizations/$ORG/ai/spend/export?columns=linear_issue_ids,github_pr_urls,repo,branch"
   ```

## Limitations

- The tool annotates the most recent interception initiated by the calling
  user. Running another AI Gateway client concurrently under the same
  account can send an annotation to the wrong interception.
- `claude mcp add-json` stores the workspace owner's session token in
  `~/.claude.json`.
- Annotation values other than server-derived capabilities are asserted by
  the client and are not verified.
