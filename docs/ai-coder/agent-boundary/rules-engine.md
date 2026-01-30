# Rules Engine Documentation

## Overview

The `rulesengine` package provides a flexible rule-based filtering system for HTTP/HTTPS requests. Rules use a simple key-value syntax with support for wildcards and multiple values.

### Basic Syntax

Rules follow the format: `key=value [key=value ...]` with three supported keys:

- **`method`**: HTTP method(s) - Any HTTP method (e.g., `GET`, `POST`, `PUT`, `DELETE`), `*` (all methods), or comma-separated list
- **`domain`**: Domain/hostname pattern - `github.com`, `*.example.com`, `*` (all domains)
- **`path`**: URL path pattern - `/api/users`, `/api/*/users`, `*` (all paths), or comma-separated list

**Key behavior**:

- If a key is omitted, it matches all values
- Multiple key-value pairs in one rule are separated by whitespace
- Multiple rules in the allowlist are OR'd together (OR logic)
- Default deny: if no rule matches, the request is denied

**Examples**:

```yaml
allowlist:
  - domain=github.com                                  # All methods, all paths for github.com (exact match)
  - domain=*.github.com                                # All subdomains of github.com
  - method=GET,POST domain=api.example.com             # GET/POST to api.example.com (exact match)
  - domain=api.example.com path=/users,/posts          # Multiple paths
  - method=GET domain=github.com path=/api/*           # All three keys
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
