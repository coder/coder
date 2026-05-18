# Rules Engine Documentation

> [!NOTE]
> Agent Firewall requires the [AI Governance Add-On](../ai-governance.md).
> As of Coder v2.32, deployments without the add-on will not be able to
> access Agent Firewall.

## Overview

The `rulesengine` package provides a flexible rule-based filtering system for
HTTP/HTTPS requests. Rules use a simple key-value syntax with support for
wildcards and multiple values.

### Basic Syntax

Rules follow the format: `key=value [key=value ...]` with three supported keys:

- **`method`**: HTTP method(s) - Any HTTP method (e.g., `GET`, `POST`, `PUT`,
  `DELETE`), `*` (all methods), or comma-separated list
- **`domain`**: Domain/hostname pattern - `github.com`, `*.example.com`, `*`
  (all domains)
- **`path`**: URL path pattern - `/api/users`, `/api/*/users`, `*` (all paths),
  or comma-separated list

**Key behavior**:

- If a key is omitted, it matches all values
- Multiple key-value pairs in one rule are separated by whitespace
- Multiple rules in the allowlist are OR'd together (OR logic)
- Default deny: if no rule matches, the request is denied

**Examples**:

```yaml
allowlist:
  - domain=github.com # All methods, all paths for github.com (exact match)
  - domain=*.github.com # All subdomains of github.com
  - method=GET,POST domain=api.example.com # GET/POST to api.example.com (exact match)
  - domain=api.example.com path=/users,/posts # Multiple paths
  - method=GET domain=github.com path=/api/* # All three keys
```

---

## Wildcard Symbol for Domains

The `*` wildcard matches domain labels (parts separated by dots).

| Pattern        | Matches                                                     | Does NOT Match                                                           |
|----------------|-------------------------------------------------------------|--------------------------------------------------------------------------|
| `*`            | All domains                                                 | -                                                                        |
| `github.com`   | `github.com` (exact match only)                             | `api.github.com`, `v1.api.github.com` (subdomains), `github.io`          |
| `*.github.com` | `api.github.com`, `v1.api.github.com` (1+ subdomain levels) | `github.com` (base domain)                                               |
| `api.*.com`    | `api.github.com`, `api.google.com`                          | `api.v1.github.com` (`*` in the middle matches exactly one domain label) |
| `*.*.com`      | `api.example.com`, `api.v1.github.com`                      | -                                                                        |
| `api.*`        | ❌ **ERROR** - Cannot end with `*`                           | -                                                                        |

**Important**:

- Patterns without `*` match **exactly** (no automatic subdomain matching)
- `*.example.com` matches one or more subdomain levels
- To match both base domain and subdomains, use separate rules:
  `domain=github.com` and `domain=*.github.com`
- Domain patterns **cannot end with asterisk**

---

## Wildcard Symbol for Paths

The `*` wildcard matches path segments (parts separated by slashes).

| Pattern        | Matches                                                    | Does NOT Match                          |
|----------------|------------------------------------------------------------|-----------------------------------------|
| `*`            | All paths                                                  | -                                       |
| `/api/users`   | `/api/users`                                               | `/api/users/123` (subpaths don't match) |
| `/api/*`       | `/api/users`, `/api/posts`                                 | `/api`                                  |
| `/api/*/users` | `/api/v1/users`, `/api/v2/users`                           | `/api/users`, `/api/v1/v2/users`        |
| `/*/users`     | `/api/users`, `/v1/users`                                  | `/api/v1/users`                         |
| `/api/v1/*`    | `/api/v1/users`, `/api/v1/users/123/details` (1+ segments) | `/api/v1`                               |

**Important**:

- `*` matches **exactly one segment** (except at the end)
- `*` at the **end** matches **one or more segments** (special behavior)
- `*` must match an entire segment (cannot be part of a segment like
  `/api/user*`)

---

## Special Meaning of Wildcard at Beginning and End

| Position   | Domain              | Path                  |
|------------|---------------------|-----------------------|
| Beginning  | 1+ subdomain levels | Exactly 1 segment     |
| Middle     | Exactly 1 label     | Exactly 1 segment     |
| End        | ❌ Not allowed       | 1+ segments (special) |
| Standalone | All domains         | All paths             |

---

## Multipath

Specify multiple paths in a single rule by separating them with commas:

```yaml
allowlist:
  - domain=api.example.com path=/users,/posts,/comments
  - domain=api.example.com path=/api,/api/*
```

`NOTE`: The pattern `/api/*` does not include the base path `/api`. To match
both, use `path=/api,/api/*`.

---

## Common policy patterns

The following examples show complete `config.yaml` allowlists for common deployment scenarios.
Each is a self-contained configuration you can adapt for your templates.

### Minimal Claude Code starter

The minimum allowlist required to run Claude Code,
based on [Anthropic's default allowed domains](https://code.claude.com/docs/en/claude-code-on-the-web#default-allowed-domains).

```yaml
allowlist:
  # Replace with your Coder deployment domain
  - domain=coder.example.com

  # Anthropic services (required)
  - domain=api.anthropic.com
  - domain=statsig.anthropic.com

  # Claude.ai (recommended - enables WebFetch and WebSearch tools)
  - domain=claude.ai

  # Error tracking (recommended - helps Anthropic diagnose issues)
  - domain=*.sentry.io
jail_type: nsjail
log_dir: /tmp/boundary_logs
log_level: warn
proxy_port: 8087
```

Every rule is load-bearing: removing `api.anthropic.com` breaks all inference,
and removing `statsig.anthropic.com` breaks feature-flag initialization.
This is the smallest policy that keeps Claude Code fully functional.
Add domains only as your agent's actual network requirements grow.

### Read-only VCS access

An agent that reads source code or calls read-only APIs should not be able to
push branches, create pull requests, or modify remote state.
Because the rules engine is a pure allowlist,
restricting writes means listing only the HTTP methods you want to permit.

```yaml
allowlist:
  # GitHub - read operations only
  - method=GET,HEAD domain=github.com
  - method=GET,HEAD domain=api.github.com
  - method=GET,HEAD domain=raw.githubusercontent.com
  - method=GET,HEAD domain=objects.githubusercontent.com

  # GitLab - read operations only
  - method=GET,HEAD domain=gitlab.com
  - method=GET,HEAD domain=api.gitlab.com

  # Required Anthropic services
  - domain=api.anthropic.com
  - domain=statsig.anthropic.com
jail_type: nsjail
log_dir: /tmp/boundary_logs
log_level: warn
proxy_port: 8087
```

`POST`, `PUT`, `PATCH`, and `DELETE` requests to these domains are denied by default
because they are absent from the allowlist.
Agent Firewall handles only HTTP/HTTPS traffic,
so SSH-based `git push` (port 22) must be blocked separately at the network level if needed.

### Path-scoped internal API

When an agent needs only a few endpoints from a broader internal service,
limit the allowlist to those specific paths rather than opening the entire host.

```yaml
allowlist:
  # Read and post results; deny all other paths on this host
  - method=GET,POST domain=internal-api.example.com path=/api/v1/results,/api/v1/results/*

  # Health check endpoint (read only)
  - method=GET domain=internal-api.example.com path=/healthz

  # Required Anthropic services
  - domain=api.anthropic.com
  - domain=statsig.anthropic.com
jail_type: nsjail
log_dir: /tmp/boundary_logs
log_level: warn
proxy_port: 8087
```

`path=/api/v1/results` matches only that exact path,
and `path=/api/v1/results/*` matches one or more segments beneath it.
Use both together (comma-separated) to cover the collection root and its members.
Any request to `internal-api.example.com` on a path not in the allowlist is denied.

### Locked-down task agent

A minimal policy for a single-purpose agent that calls one AI provider and one
internal endpoint, with no broader internet access.
Use this pattern for automated tasks where minimizing the blast radius of a
compromised or misbehaving agent is a priority.

```yaml
allowlist:
  # Required Anthropic services
  - domain=api.anthropic.com
  - domain=statsig.anthropic.com

  # One specific internal endpoint, write-only
  - method=POST domain=tasks.internal.example.com path=/run

  # Coder deployment (required for agent heartbeat and log streaming)
  - domain=coder.example.com
jail_type: nsjail
log_dir: /tmp/boundary_logs
log_level: info
proxy_port: 8087
```

`log_level: info` records every request, not just blocked ones.
This is recommended for automated tasks where a full request history may be needed for incident review.
Once the policy is stable and request volume is high,
switch to `log_level: warn` to reduce log storage costs.
The agent has no access to package registries, CDNs, or any other external host.
If the task ever needs a new network dependency, the allowlist must be explicitly updated.
