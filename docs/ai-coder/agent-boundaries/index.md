# Agent Boundaries

Agent Boundaries are process-level firewalls that restrict and audit what
autonomous programs — such as AI coding agents — can access over the network.
They use a **default-deny architecture** where administrators explicitly
allowlist which domains, HTTP methods, and URL paths agents can reach. Everything
else is blocked and logged.

![Screenshot of Agent Boundaries blocking a process](../../images/guides/ai-agents/boundary.png)Example
of Agent Boundaries blocking a process.

## Architecture

Agent Boundaries use Linux kernel-level isolation to create a separate network
namespace around the wrapped process. All outbound network requests from the
agent are routed through a proxy where the allowlist policy is applied.

### How it works

Boundary operates at the **process level**, not at the workspace or VM level. It
wraps individual agent processes (like `claude` or `codex`) and ensures that the
wrapped process can only reach the network through Boundary's proxy.

### Enforcement lifecycle

1. A template admin defines a `config.yaml` with network policies (allowed
   domains, HTTP methods, URL paths).
2. The config is mounted into the workspace via a `coder_script` resource.
3. Agent Boundaries wraps the agent process:
   `boundary -- claude` (or via the Claude Code module with
   `enable_boundary = true`).
4. All outbound network requests from the wrapped process are intercepted.
5. Each request is checked against the policy (URL pattern + HTTP method).
6. Allowed requests pass through; blocked requests are denied.
7. Audit logs (policy decisions) stream to the Coder control plane for
   centralized monitoring.
8. Application logs (debugging info) are written locally to the workspace.

### Key architectural points

- **Process-level isolation.** Boundary wraps individual processes, not entire
  workspaces. The agent cannot reach the network except through Boundary's
  proxy.
- **Default-deny.** Requests are blocked unless permitted by the allowlist.
- **Template-level governance.** Policies are defined in workspace templates
  (infrastructure as code), not per-user. Every workspace launched from a
  template version picks up the same policy.
- **Agent-agnostic.** Works with any terminal-based agent, including Claude Code,
  Codex, and custom agents.

## Supported agents

Agent Boundaries work with any terminal-based agent that runs inside a Coder
workspace, including:

- Claude Code
- Codex CLI
- Custom agents and scripts

> **Note:** Agent Boundaries only protect agents running inside Coder
> workspaces. IDE-based agents running on a developer's local machine (such as
> Cursor running locally) cannot be wrapped by Boundary — the agent's runtime
> must be inside the workspace for Boundary to be effective.

## Features

- **Data exfiltration prevention**: Agents can only reach explicitly allowed
  domains. Unauthorized destinations (e.g., pastebin, public S3 buckets) are
  blocked by default.
- **Prompt injection mitigation**: Even if an agent is tricked by malicious
  content (e.g., a crafted README), Boundary limits what network actions the
  agent can actually take.
- **Supply chain protection**: Control which package registries agents can pull
  from.
- **Centralized audit logs**: All policy decisions (allow/deny) stream to the
  Coder control plane as structured logs, available for any log aggregation
  system.
- **Template-level governance**: Policies travel with the template — define
  once, apply to every workspace.

## Getting started

There are two ways to adopt Agent Boundaries. We recommend doing both simultaneously.

### Via Coder modules 

Enable Agent Boundaries in existing agent modules. The module handles
installation and wrapping automatically, including within
[Coder Tasks](../tasks.md).

```hcl
module "claude-code" {
  source           = "dev.registry.coder.com/coder/claude-code/coder"
  version          = "4.7.0"
  enable_boundary  = true
}
```

### Via standalone CLI

Install the `boundary` CLI directly and wrap any terminal-based agent manually:

```bash
curl -fsSL https://raw.githubusercontent.com/coder/boundary/main/install.sh | bash
```

Then wrap your agent:

```bash
boundary -- claude
boundary -- codex
boundary -- ./my-custom-agent.sh
```

This approach is necessary when developers SSH into workspaces and run agents
directly rather than through Coder Tasks.

## Configuration

> For version requirements and compatibility, see the
> [Version Requirements](./version.md) documentation.

Agent Boundaries are configured using a `config.yaml` file placed at
`~/.config/coder_boundary/config.yaml`. This allows you to maintain allowlists
and share detailed policies with teammates through version control.

In your Terraform module, enable Agent Boundaries with minimal configuration:

```tf
module "claude-code" {
  source              = "dev.registry.coder.com/coder/claude-code/coder"
  version             = "4.7.0"
  enable_boundary     = true
}
```

### Minimal configuration

Create a `config.yaml` file in your template directory:

For the Claude Code module, use the following minimal configuration:

```yaml
allowlist:
  - "domain=dev.coder.com" # Required - use your Coder deployment domain
  - "domain=api.anthropic.com" # Required - API endpoint for Claude
  - "domain=statsig.anthropic.com" # Required - Feature flags and analytics
  - "domain=claude.ai" # Recommended - WebFetch/WebSearch features
  - "domain=*.sentry.io" # Recommended - Error tracking (helps Anthropic fix bugs)
log_dir: /tmp/boundary_logs
proxy_port: 8087
log_level: warn
```

For a recommended starting point, see the
[Anthropic documentation on default allowed domains](https://code.claude.com/docs/en/claude-code-on-the-web#default-allowed-domains).
For a comprehensive production example, see the
[Coder dogfood policy](https://github.com/coder/coder/blob/main/dogfood/coder/boundary-config.yaml).

### Mounting the configuration

Add a `coder_script` resource to mount the config into the workspace:

```tf
resource "coder_script" "boundary_config_setup" {
  agent_id     = coder_agent.dev.id
  display_name = "Boundary Setup Configuration"
  run_on_start = true

  script = <<-EOF
    #!/bin/sh
    mkdir -p ~/.config/coder_boundary
    echo '${base64encode(file("${path.module}/config.yaml"))}' | base64 -d > ~/.config/coder_boundary/config.yaml
    chmod 600 ~/.config/coder_boundary/config.yaml
  EOF
}
```

Agent Boundaries automatically reads `config.yaml` from
`~/.config/coder_boundary/` when it starts. Everyone who launches Agent
Boundaries inside the workspace picks up the same configuration without extra
flags. This is especially convenient for managing extensive allow lists in
version control.

### Configuration parameters

| Parameter    | Description                                                      |
|------------- |------------------------------------------------------------------|
| `allowlist`  | URL patterns the agent can access. See [allowlist rules](#allowlist-rules) below. |
| `log_dir`    | Directory where Boundary writes local log files.                 |
| `log_level`  | Verbosity: `WARN` (blocked only), `INFO` (all requests), `DEBUG` (detailed). |
| `proxy_port` | Port used by the HTTP proxy.                                     |

### Allowlist rules

Rules use the format `"key=value [key=value ...]"`:

| Pattern | Description |
|---------|-------------|
| `domain=github.com` | Allows the domain and all its subdomains. |
| `domain=*.github.com` | Allows only subdomains (the specific domain is excluded). |
| `method=GET,HEAD domain=api.github.com` | Allows specific HTTP methods for a domain. |
| `method=POST domain=api.example.com path=/users,/posts` | Allows specific methods, domain, and paths. |
| `path=/api/v1/*,/api/v2/*` | Allows specific URL paths. |

For detailed information about rule construction, see the
[rules engine documentation](./rules-engine.md).

You can also run Agent Boundaries directly in your workspace and configure it
per template. You can do so by installing the
[binary](https://github.com/coder/boundary) into the workspace image or at
start-up. You can do so with the following command:

```bash
curl -fsSL https://raw.githubusercontent.com/coder/boundary/main/install.sh | bash
```

## Jail Types

Agent Boundaries supports two jail types for process isolation:

### nsjail (default)

Uses Linux namespaces for isolation. Creates a separate network namespace at the
kernel level and routes all traffic through the proxy. Requires `CAP_NET_ADMIN`.

See [nsjail documentation](./nsjail.md) for runtime requirements and Docker
configuration.

### landjail

Uses Landlock V4 for network isolation. **Requires no special permissions**,
making it suitable for environments where granting `CAP_NET_ADMIN` is not
feasible.

See [landjail documentation](./landjail.md) for implementation details.

### Choosing a jail type

The choice of jail type depends on your security requirements, available Linux
capabilities, and runtime environment. Both nsjail and landjail provide network
isolation, but they use different underlying mechanisms. nsjail uses Linux
namespaces, while landjail uses Landlock V4. Landjail may be preferred in
environments where namespace capabilities are limited or unavailable.

## Implementation Comparison: Namespaces+iptables vs Landlock V4

| Aspect                        | Namespace Jail (Namespaces + veth-pair + iptables)                                | Landlock V4 Jail                                                        |
|-------------------------------|-----------------------------------------------------------------------------------|-------------------------------------------------------------------------|
| **Privileges**                | Requires `CAP_NET_ADMIN`                                                          | ✅ No special capabilities required                                      |
| **Docker seccomp**            | ❌ Requires seccomp profile modifications or sysbox-runc                           | ✅ Works without seccomp changes                                         |
| **Kernel requirements**       | Linux 3.8+ (widely available)                                                     | ❌ Linux 6.7+ (very new, limited adoption)                               |
| **Bypass resistance**         | ✅ Strong - transparent interception prevents bypass                               | ❌ **Medium - can bypass by connecting to `evil.com:<HTTP_PROXY_PORT>`** |
| **Process isolation**         | ✅ PID namespace (processes can't see/kill others); **implementation in-progress** | ❌ No PID namespace (agent can kill other processes)                     |
| **Non-TCP traffic control**   | ✅ Can block/control UDP via iptables; **implementation in-progress**              | ❌ No control over UDP (data can leak via UDP)                           |
| **Application compatibility** | ✅ Works with ANY application (transparent interception)                           | ❌ Tools without `HTTP_PROXY` support will be blocked                    |

## Audit Logs

Agent Boundaries stream audit logs to the Coder control plane, providing
centralized visibility into HTTP requests made within workspaces—whether from AI
agents or ad-hoc commands run with `boundary`.

Audit logs are independent of application logs:

- **Audit logs** record Agent Boundaries' policy decisions: whether each HTTP
  request was allowed or denied based on the allowlist rules. These are always
  sent to the control plane regardless of Agent Boundaries' configured log
  level.
- **Application logs** are Agent Boundaries' operational logs written locally to
  the workspace. These include startup messages, internal errors, and debugging
  information controlled by the `log_level` setting.

For example, if a request to `api.example.com` is allowed by Agent Boundaries
but the remote server returns a 500 error, the audit log records
`decision=allow` because Agent Boundaries permitted the request. The HTTP
response status is not tracked in audit logs.

> [!NOTE]
> Requires Coder v2.30+ and Agent Boundaries v0.5.2+.

### Audit Log Contents

Each Agent Boundaries audit log entry includes:

| Field                 | Description                                                                             |
|-----------------------|-----------------------------------------------------------------------------------------|
| `decision`            | Whether the request was allowed (`allow`) or blocked (`deny`)                           |
| `workspace_id`        | The UUID of the workspace where the request originated                                  |
| `workspace_name`      | The name of the workspace where the request originated                                  |
| `owner`               | The owner of the workspace where the request originated                                 |
| `template_id`         | The UUID of the template that the workspace was created from                            |
| `template_version_id` | The UUID of the template version used by the current workspace build                    |
| `http_method`         | The HTTP method used (GET, POST, PUT, DELETE, etc.)                                     |
| `http_url`            | The fully qualified URL that was requested                                              |
| `event_time`          | Timestamp when boundary processed the request (RFC3339 format)                          |
| `matched_rule`        | The allowlist rule that permitted the request (only present when `decision` is `allow`) |

### Viewing Audit Logs

Audit logs are emitted as structured log entries from the Coder server. Collect
and analyze them using any log aggregation system (Grafana Loki, Splunk,
Elastic, etc.)

Example of an allowed request (assuming stderr):

```console
2026-01-16 00:11:40.564 [info]  coderd.agentrpc: boundary_request owner=joe  workspace_name=some-task-c88d agent_name=dev  decision=allow  workspace_id=f2bd4e9f-7e27-49fc-961e-be4d1c2aa987  http_method=GET http_url=https://dev.coder.com  event_time=2026-01-16T00:11:39.388607657Z  matched_rule=domain=dev.coder.com request_id=9f30d667-1fc9-47ba-b9e5-8eac46e0abef trace=478b2b45577307c4fd1bcfc64fad6ffb span=9ece4bc70c311edb
```
## Using Agent Boundaries with AI Bridge

Agent Boundaries and [AI Bridge](../ai-bridge/index.md) are independent features
that deliver maximum value when deployed together:

- **AI Bridge** governs the *LLM layer* — identity, audit, cost, and tool
  management.
- **Agent Boundaries** governs the *network layer* — restricting which domains
  agents can reach.

Because Boundaries use a default-deny architecture, agent processes wrapped by Boundary cannot reach LLM provider APIs directly unless those domains are explicitly allowlisted. Organizations using AI Bridge can omit provider domains from the Boundary allowlist to ensure that all AI traffic routes through Bridge.

## Current scope and limitations

Agent Boundaries are under active development. The following describes the
current scope to help you plan your deployment:

- **Process-level, not network-level.** Boundary wraps individual processes. If
  an agent is launched directly without the `boundary` wrapper (e.g., `claude`
  instead of `boundary -- claude`), it will not be subject to Boundary policies.
  To enforce Boundary usage, run agents through
  [Coder Tasks](../tasks.md), where Boundary can be required by the template.
- **Per-template scoping.** Policies are scoped per-template, not per-user or
  per-group. To apply different policies to different teams, use separate
  templates. Policies are updated when workspaces are rebuilt with the latest
  template version.
- **CLI-based agents only.** Boundary protects agents running inside Coder
  workspaces. It does not protect IDE-based agents running on a developer's
  local machine — the agent runtime must be inside the workspace.
- **No agent source discrimination.** Boundary currently cannot differentiate
  between requests originating from a user command versus an AI agent within the
  same wrapped process. To apply different trust levels, use separate templates
  (e.g., a human+agent template with broader access vs. an agent-only template
  with restricted access).
- **Log correlation with AI Bridge.** Correlating Boundary audit logs with AI
  Bridge logs currently requires exporting both streams to an external SIEM and
  joining on shared fields (user, workspace, timestamp). Built-in
  cross-referencing is planned for a future release.

## Next steps

- [nsjail](./nsjail.md) — Namespace-based jail configuration and requirements.
- [landjail](./landjail.md) — Landlock V4-based jail without extra permissions.
- [Rules Engine](./rules-engine.md) — Detailed guide to constructing allowlist
  rules.
- [Version Compatibility](./version.md) — Version requirements for Coder and
  Agent Boundaries.
