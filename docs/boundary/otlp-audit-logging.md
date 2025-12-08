# Boundary OTLP Audit Logging

Boundary can export network access audit logs to any OTLP (OpenTelemetry Protocol) compatible endpoint, enabling centralized monitoring and alerting on workspace network activity.

## Configuration

Configure OTLP logging in your `boundary-config.yaml` file. See [boundary-config.yaml](./boundary-config.yaml) for a complete example.

```yaml
# OTLP Audit Logging
otlp_endpoint: "https://otel-collector.example.com:4318/v1/logs"
otlp_headers: "x-api-key=your-api-key"

# Workspace metadata (included in log attributes)
workspace_id: "abc-123"
workspace_name: "my-workspace"
workspace_owner: "alice"
```

## Configuration Reference

| Option | Environment Variable | Description |
|--------|---------------------|-------------|
| `otlp_endpoint` | `BOUNDARY_OTLP_ENDPOINT` | OTLP HTTP endpoint URL |
| `otlp_headers` | `BOUNDARY_OTLP_HEADERS` | Comma-separated `key=value` headers |
| `otlp_insecure` | `BOUNDARY_OTLP_INSECURE` | Skip TLS verification (not recommended) |
| `otlp_ca_cert` | `BOUNDARY_OTLP_CA_CERT` | Path to CA certificate for custom CAs |
| `workspace_id` | `BOUNDARY_WORKSPACE_ID` | Coder workspace ID |
| `workspace_name` | `BOUNDARY_WORKSPACE_NAME` | Coder workspace name |
| `workspace_owner` | `BOUNDARY_WORKSPACE_OWNER` | Coder workspace owner |

## TLS Configuration

| Scenario | Configuration |
|----------|---------------|
| Public endpoint (system CAs) | Just set `otlp_endpoint: https://...` |
| Internal/custom CA | Set `otlp_ca_cert: /path/to/ca.crt` |
| Development (no TLS) | Set `otlp_endpoint: http://...` or `otlp_insecure: true` |

## Log Record Format

Each OTLP log record contains:

| Attribute | Description | Example |
|-----------|-------------|---------|
| `timestamp` | Request timestamp | `2024-01-15T10:30:00Z` |
| `severity` | INFO (allow) or WARN (deny) | `INFO` |
| `body` | Event type | `network_access` |
| `resource.service.name` | Service identifier | `boundary` |
| `decision` | Access decision | `allow` or `deny` |
| `http.method` | HTTP method | `GET`, `POST`, `CONNECT` |
| `http.url` | Requested URL | `https://api.github.com/user` |
| `http.host` | Target host | `api.github.com` |
| `rule` | Matched rule (if any) | `domain=github.com` |
| `workspace.id` | Coder workspace ID | `abc-123` |
| `workspace.name` | Coder workspace name | `my-workspace` |
| `workspace.owner` | Coder workspace owner | `alice` |

## Multiple Auditors

When OTLP is enabled, **stdout logging remains active**. This ensures you can always see audit logs locally while also exporting to your observability platform.

| Configuration | Active Auditors |
|--------------|-----------------|
| Default | LogAuditor (stdout) |
| `audit_socket` set | LogAuditor + SocketAuditor |
| `otlp_endpoint` set | LogAuditor + OTLPAuditor |
| Both set | LogAuditor + SocketAuditor + OTLPAuditor |

## Examples

### Honeycomb

```yaml
otlp_endpoint: "https://api.honeycomb.io/v1/logs"
otlp_headers: "x-honeycomb-team=YOUR_API_KEY"
```

### Grafana Cloud

```yaml
otlp_endpoint: "https://otlp-gateway-prod-us-central-0.grafana.net/otlp/v1/logs"
otlp_headers: "Authorization=Basic BASE64_ENCODED_CREDENTIALS"
```

### Self-hosted Collector with Internal CA

```yaml
otlp_endpoint: "https://otel-collector.internal:4318/v1/logs"
otlp_ca_cert: "/etc/ssl/certs/internal-ca.crt"
```
