---
title: Rebranding Migration
---

AI Bridge has been renamed to **AI Gateway**. This is a cosmetic rebrand to make
the feature easier to understand. It changes user-visible names, configuration
options, the canonical HTTP API path, and the Prometheus metric names.

> [!NOTE]
> Deprecated aliases remain available for gateway and proxy controls.
> Provider configuration through environment variables, flags, and YAML has been removed under both names; refer to [Provider configuration](./providers.md#database-management-of-providers).

Adopt the `ai_gateway` names for gateway and proxy settings so your configuration matches the current documentation.

> [!IMPORTANT]
> New settings added in every area except the database (configuration options,
> environment variables, CLI flags, and API paths) will use only the new
> `ai_gateway` name, with no `aibridge` alias.

## At a glance

| Area                  | Old name                                       | New (canonical) name                              | Old name still works?              |
|-----------------------|------------------------------------------------|---------------------------------------------------|------------------------------------|
| Environment variables | `CODER_AIBRIDGE_*`                             | `CODER_AI_GATEWAY_*`                              | Yes, except provider setup         |
| CLI flags             | `--aibridge-*`                                 | `--ai-gateway-*`                                  | Yes, except provider setup         |
| YAML config group     | `aibridge:` / `aibridgeproxy:`                 | `ai_gateway:` / `ai_gateway_proxy:`               | Yes, except provider setup         |
| HTTP API              | `/api/v2/aibridge`                             | `/api/v2/ai-gateway`                              | Yes (legacy route retained)        |
| Prometheus metrics    | `coder_aibridged_*` / `coder_aibridgeproxyd_*` | `coder_ai_gateway_*` / `coder_ai_gateway_proxy_*` | Yes (both emitted, old deprecated) |
| Database              | (no change)                                    | (no change)                                       | n/a                                |

## What did not change

- **No database changes.** Table and column names (for example,
  `aibridge_interceptions`) are unchanged. No migration runs and no data is
  rewritten on upgrade.
- **Gateway and proxy settings.** Renaming options for logging, retention, or listener addresses does not change their behavior or defaults.
- **Internal/library references.** Some internal package names, log fields, and
  library identifiers still use the `aibridge` name. These are not part of the
  supported configuration surface and do not affect operators.

## Configuration (env vars, flags, YAML)

The aliases listed below apply only to gateway and proxy controls, not provider setup.

If both a previous name and a new name are set for the same setting, set only one (prefer
the new name).

### Naming rules

The rename is a mechanical substitution:

- Environment variables: `CODER_AIBRIDGE_` becomes `CODER_AI_GATEWAY_`.
- CLI flags: `--aibridge-` becomes `--ai-gateway-`.
- YAML: only the top-level group key changes
  (`aibridge:` becomes `ai_gateway:`, `aibridgeproxy:` becomes
  `ai_gateway_proxy:`). The keys nested under the group are unchanged.

### YAML example

Before:

```yaml
aibridge:
  enabled: true
  retention: 60d
aibridgeproxy:
  enabled: true
  listen_addr: ":8888"
```

After:

```yaml
ai_gateway:
  enabled: true
  retention: 60d
ai_gateway_proxy:
  enabled: true
  listen_addr: ":8888"
```

### Environment variable reference

Core AI Gateway settings:

| Deprecated                                         | New                                                  | Note |
|----------------------------------------------------|------------------------------------------------------|------|
| `CODER_AIBRIDGE_ENABLED`                           | `CODER_AI_GATEWAY_ENABLED`                           |      |
| `CODER_AIBRIDGE_INJECT_CODER_MCP_TOOLS`            | `CODER_AI_GATEWAY_INJECT_CODER_MCP_TOOLS`            |      |
| `CODER_AIBRIDGE_RETENTION`                         | `CODER_AI_GATEWAY_RETENTION`                         |      |
| `CODER_AIBRIDGE_MAX_CONCURRENCY`                   | `CODER_AI_GATEWAY_MAX_CONCURRENCY`                   |      |
| `CODER_AIBRIDGE_RATE_LIMIT`                        | `CODER_AI_GATEWAY_RATE_LIMIT`                        |      |
| `CODER_AIBRIDGE_STRUCTURED_LOGGING`                | `CODER_AI_GATEWAY_STRUCTURED_LOGGING`                |      |
| `CODER_AIBRIDGE_SEND_ACTOR_HEADERS`                | `CODER_AI_GATEWAY_SEND_ACTOR_HEADERS`                |      |
| `CODER_AIBRIDGE_ALLOW_BYOK`                        | `CODER_AI_GATEWAY_ALLOW_BYOK`                        |      |
| `CODER_AIBRIDGE_CIRCUIT_BREAKER_ENABLED`           | `CODER_AI_GATEWAY_CIRCUIT_BREAKER_ENABLED`           |      |
| `CODER_AIBRIDGE_CIRCUIT_BREAKER_FAILURE_THRESHOLD` | `CODER_AI_GATEWAY_CIRCUIT_BREAKER_FAILURE_THRESHOLD` |      |
| `CODER_AIBRIDGE_CIRCUIT_BREAKER_INTERVAL`          | `CODER_AI_GATEWAY_CIRCUIT_BREAKER_INTERVAL`          |      |
| `CODER_AIBRIDGE_CIRCUIT_BREAKER_TIMEOUT`           | `CODER_AI_GATEWAY_CIRCUIT_BREAKER_TIMEOUT`           |      |
| `CODER_AIBRIDGE_CIRCUIT_BREAKER_MAX_REQUESTS`      | `CODER_AI_GATEWAY_CIRCUIT_BREAKER_MAX_REQUESTS`      |      |

AI Gateway Proxy settings:

| Deprecated                                   | New                                            | Note                              |
|----------------------------------------------|------------------------------------------------|-----------------------------------|
| `CODER_AIBRIDGE_PROXY_ENABLED`               | `CODER_AI_GATEWAY_PROXY_ENABLED`               |                                   |
| `CODER_AIBRIDGE_PROXY_LISTEN_ADDR`           | `CODER_AI_GATEWAY_PROXY_LISTEN_ADDR`           |                                   |
| `CODER_AIBRIDGE_PROXY_TLS_CERT_FILE`         | `CODER_AI_GATEWAY_PROXY_TLS_CERT_FILE`         |                                   |
| `CODER_AIBRIDGE_PROXY_TLS_KEY_FILE`          | `CODER_AI_GATEWAY_PROXY_TLS_KEY_FILE`          |                                   |
| `CODER_AIBRIDGE_PROXY_CERT_FILE`             | `CODER_AI_GATEWAY_PROXY_CERT_FILE`             |                                   |
| `CODER_AIBRIDGE_PROXY_KEY_FILE`              | `CODER_AI_GATEWAY_PROXY_KEY_FILE`              |                                   |
| `CODER_AIBRIDGE_PROXY_UPSTREAM`              | `CODER_AI_GATEWAY_PROXY_UPSTREAM`              |                                   |
| `CODER_AIBRIDGE_PROXY_UPSTREAM_CA`           | `CODER_AI_GATEWAY_PROXY_UPSTREAM_CA`           |                                   |
| `CODER_AIBRIDGE_PROXY_ALLOWED_PRIVATE_CIDRS` | `CODER_AI_GATEWAY_PROXY_ALLOWED_PRIVATE_CIDRS` |                                   |
| `CODER_AIBRIDGE_PROXY_DUMP_DIR`              | `CODER_AI_GATEWAY_PROXY_DUMP_DIR`              |                                   |
| `CODER_AIBRIDGE_PROXY_DOMAIN_ALLOWLIST`      | `CODER_AI_GATEWAY_PROXY_DOMAIN_ALLOWLIST`      | Already deprecated; has no effect |

CLI flags follow the same mapping with the `--aibridge-*` to `--ai-gateway-*`
prefix change.

## HTTP API

The canonical API path is now `/api/v2/ai-gateway` (and
`/api/v2/ai-gateway/proxy`). The legacy `/api/v2/aibridge` and
`/api/v2/aibridge/proxy` routes are retained for backward compatibility and
continue to serve the same handlers.

If you have external integrations or agents calling the API directly, update them to the
new path at your convenience. No immediate action is required.

AI clients (such as Claude Code, Codex, and other tools) that are configured
with a base URL pointing at the legacy `/api/v2/aibridge` path continue to work,
but should be updated to the new `/api/v2/ai-gateway` base URL.

## Metrics

The metric prefixes have been renamed:

| Deprecated prefix        | New prefix                 |
|--------------------------|----------------------------|
| `coder_aibridged_*`      | `coder_ai_gateway_*`       |
| `coder_aibridgeproxyd_*` | `coder_ai_gateway_proxy_*` |

**Both the old and new metric names are emitted simultaneously today.** Every
series is exported under both prefixes from the same underlying collector, so
existing dashboards, alerts, and recording rules keep working immediately after
upgrade with no changes.

The old prefixes are retained for backward compatibility with no planned removal
date. To keep your observability aligned with the new names:

1. Update Grafana dashboards, Prometheus alerting rules, and recording rules to
   reference the new `coder_ai_gateway_*` and `coder_ai_gateway_proxy_*` names.
2. Verify the new series are present in your monitoring stack (they are emitted
   as of this release).

### Optional: dropping the old names

If you have already migrated to the new names and do not want both prefixes
ingested (for example, to avoid doubling cardinality in your time-series
database), you can drop the deprecated series at scrape time with Prometheus
`metric_relabel_configs`:

```yaml
metric_relabel_configs:
  - source_labels: [__name__]
    regex: 'coder_aibridged_.*|coder_aibridgeproxyd_.*'
    action: drop
```

This keeps the canonical `coder_ai_gateway_*` and `coder_ai_gateway_proxy_*`
series and discards the deprecated `coder_aibridged_*` and
`coder_aibridgeproxyd_*` ones before they are stored. Only do this once your
dashboards, alerts, and recording rules reference the new names.
