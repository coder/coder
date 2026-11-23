---
title: Rules engine documentation
---

> [!NOTE]
> Agent Firewall is part of [AI Governance](../index.md), which is included with a Premium license.

## Overview

The `rulesengine` package provides a flexible rule-based filtering system for HTTP/HTTPS requests.
Rules use a simple key-value syntax with support for wildcards and multiple values.

### Basic Syntax

Rules follow the format: `key=value [key=value ...]` with three supported keys:

- **`method`**:
  HTTP method(s) - Any HTTP method (for example, `GET`, `POST`, `PUT`, `DELETE`), `*` (all methods), or comma-separated list
- **`domain`**:
  Domain/hostname pattern - `github.com`, `*.example.com`, `*` (all domains)
- **`path`**:
  URL path pattern - `/api/users`, `/api/*/users`, `*` (all paths), or comma-separated list

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
- To match both base domain and subdomains, use separate rules: `domain=github.com` and `domain=*.github.com`
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
- `*` must match an entire segment (cannot be part of a segment like `/api/user*`)

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

`NOTE`: The pattern `/api/*` does not include the base path `/api`.
To match both, use `path=/api,/api/*`.

---

## Common policy patterns

The following examples show `allowlist` rules for common scenarios.
Each block contains only the `allowlist` key, so merge it into your `config.yaml` with the other settings you use.
For a broad multi-language policy, refer to the [example policy](./example-policy.md).

### Read-only access to a code host

```yaml
allowlist:
  # Read-only access to the GitHub REST API.
  - method=GET,HEAD domain=api.github.com
  # Read-only Git over HTTPS on github.com, with or without the .git suffix.
  - method=GET domain=github.com path=/*/*/info/refs
  - method=POST domain=github.com path=/*/*/git-upload-pack
```

The first rule allows `GET` and `HEAD` requests to `api.github.com`, such as `GET https://api.github.com/repos/coder/coder`, and denies `POST`, `PATCH`, `PUT`, and `DELETE` to that host.
The other two rules allow the Git smart HTTP requests that a clone or fetch sends to `github.com`, and deny other requests to that host.
Web pages such as `https://github.com/coder/coder` are denied because they match neither path pattern.
Git sends the changes in a push as a `POST` to `git-receive-pack`, which this policy denies, so the push fails.

### Path-scoped internal API

```yaml
allowlist:
  # Read and write the results collection and the items beneath it.
  - method=GET,POST domain=internal-api.example.com path=/api/v1/results,/api/v1/results/*
  # Health check endpoint, read only.
  - method=GET domain=internal-api.example.com path=/healthz
```

This policy allows `GET` and `POST` to `/api/v1/results` and to paths beneath it, such as `/api/v1/results/42/logs`.
It denies `DELETE` on those paths and requests to other paths on the host, such as `/api/v1/users`, except the health check.
The pattern `/api/v1/results/*` does not match the collection root, so list both patterns.
A `path` rule without a `domain` matches every host, so always pair `path` with `domain`.

### Base domain and subdomains

```yaml
allowlist:
  # The company domain and every subdomain beneath it.
  - domain=example.com
  - domain=*.example.com
```

The rule `domain=example.com` matches only `example.com`, and `domain=*.example.com` matches `build.example.com` and `a.b.example.com` but not `example.com`.
Neither rule matches `badexample.com` or `example.com.attacker.net`, so listing both rules covers the domain without opening lookalike hosts.

### Locked-down task agent

```yaml
allowlist:
  # Model provider API, all methods.
  - domain=api.anthropic.com
  # Task service, POST to the run endpoint only.
  - method=POST domain=tasks.internal.example.com path=/run
```

This policy allows requests to the model provider and `POST /run` on the task service.
It denies `GET` requests to the task service, `POST` to other paths such as `/run/abort`, and every other host, including package registries.
With this policy, the agent's HTTP requests can reach only the model provider and the run endpoint.
