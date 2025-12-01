# Data Retention

Coder supports configurable retention policies that automatically purge old
Audit Logs, Connection Logs, Workspace Agent Logs, and API keys. These policies
help manage database growth by removing records older than a specified duration.

## Overview

Large deployments can accumulate significant amounts of data over time.
Retention policies help you:

- **Reduce database size**: Automatically remove old records to free disk space.
- **Improve performance**: Smaller tables mean faster queries and backups.
- **Meet compliance requirements**: Configure retention periods that align with
  your organization's data retention policies.

> [!NOTE]
> Retention policies are disabled by default (set to `0`) to preserve existing
> behavior. The exceptions are API keys and workspace agent logs, which default
> to 7 days.

## Configuration

You can configure retention policies using CLI flags, environment variables, or
a YAML configuration file.

### Settings

| Setting              | CLI Flag                           | Environment Variable                   | Default        | Description                              |
|----------------------|------------------------------------|----------------------------------------|----------------|------------------------------------------|
| Audit Logs           | `--audit-logs-retention`           | `CODER_AUDIT_LOGS_RETENTION`           | `0` (disabled) | How long to retain Audit Log entries     |
| Connection Logs      | `--connection-logs-retention`      | `CODER_CONNECTION_LOGS_RETENTION`      | `0` (disabled) | How long to retain Connection Logs       |
| API Keys             | `--api-keys-retention`             | `CODER_API_KEYS_RETENTION`             | `7d`           | How long to retain expired API keys      |
| Workspace Agent Logs | `--workspace-agent-logs-retention` | `CODER_WORKSPACE_AGENT_LOGS_RETENTION` | `7d`           | How long to retain workspace agent logs  |

### Duration Format

Retention durations support days (`d`) and weeks (`w`) in addition to standard
Go duration units (`h`, `m`, `s`):

- `7d` - 7 days
- `2w` - 2 weeks
- `30d` - 30 days
- `90d` - 90 days
- `365d` - 1 year

### CLI Example

```bash
coder server \
  --audit-logs-retention=365d \
  --connection-logs-retention=90d \
  --api-keys-retention=7d \
  --workspace-agent-logs-retention=7d
```

### Environment Variables Example

```bash
export CODER_AUDIT_LOGS_RETENTION=365d
export CODER_CONNECTION_LOGS_RETENTION=90d
export CODER_API_KEYS_RETENTION=7d
export CODER_WORKSPACE_AGENT_LOGS_RETENTION=7d
```

### YAML Configuration Example

```yaml
retention:
  audit_logs: 365d
  connection_logs: 90d
  api_keys: 7d
  workspace_agent_logs: 7d
```

## How Retention Works

### Background Purge Process

Coder runs a background process that periodically deletes old records. The
purge process:

1. Runs approximately every 10 minutes.
2. Processes records in batches to avoid database lock contention.
3. Deletes records older than the configured retention period.
4. Logs the number of deleted records for monitoring.

### Effective Retention

Each retention setting controls its data type independently:

- When set to a non-zero duration, records older than that duration are deleted.
- When set to `0`, retention is disabled and data is kept indefinitely.

### API Keys Special Behavior

API key retention only affects **expired** keys. A key is deleted only when:

1. The key has expired (past its `expires_at` timestamp).
2. The key has been expired for longer than the retention period.

Setting `--api-keys-retention=7d` deletes keys that expired more than 7 days
ago. Active keys are never deleted by the retention policy.

Keeping expired keys for a short period allows Coder to return a more helpful
error message when users attempt to use an expired key.

### Workspace Agent Logs Behavior

Workspace agent logs are retained based on the retention period, but **logs from
the latest build of each workspace are always retained** regardless of age. This
ensures you can always debug issues with active workspaces.

Only logs from non-latest workspace builds that are older than the retention
period are deleted. Setting `--workspace-agent-logs-retention=7d` keeps all logs
from the latest build plus logs from previous builds for up to 7 days.

## Best Practices

### Recommended Starting Configuration

For most deployments, we recommend:

```yaml
retention:
  audit_logs: 365d
  connection_logs: 90d
  api_keys: 7d
  workspace_agent_logs: 7d
```

### Compliance Considerations

> [!WARNING]
> Audit Logs provide critical security and compliance information. Purging
> Audit Logs may impact your organization's ability to investigate security
> incidents or meet compliance requirements. Consult your security and
> compliance teams before configuring Audit Log retention.

Common compliance frameworks have varying retention requirements:

- **SOC 2**: Typically requires 1 year of audit logs.
- **HIPAA**: Requires 6 years for certain records.
- **PCI DSS**: Requires 1 year of audit logs, with 3 months immediately
  available.
- **GDPR**: Requires data minimization but does not specify maximum retention.

### External Log Aggregation

If you use an external log aggregation system (Splunk, Datadog, etc.), you can
configure shorter retention periods in Coder since logs are preserved
externally. See
[Capturing/Exporting Audit Logs](../security/audit-logs.md#capturingexporting-audit-logs)
for details on exporting logs.

### Database Maintenance

After enabling retention policies, you may want to run a `VACUUM` operation on
your PostgreSQL database to reclaim disk space. See
[Maintenance Procedures](../security/audit-logs.md#maintenance-procedures-for-the-audit-logs-table)
for guidance.

## Keeping Data Indefinitely

To keep data indefinitely for any data type, set its retention value to `0`:

```yaml
retention:
  audit_logs: 0s           # Keep audit logs forever
  connection_logs: 0s      # Keep connection logs forever
  api_keys: 0s             # Keep expired API keys forever
  workspace_agent_logs: 0s # Keep workspace agent logs forever
```

## Monitoring

The purge process logs deletion counts at the `DEBUG` level. To monitor
retention activity, enable debug logging or search your logs for entries
containing the table name (e.g., `audit_logs`, `connection_logs`, `api_keys`).

## Related Documentation

- [Audit Logs](../security/audit-logs.md): Learn about Audit Logs and manual
  purge procedures.
- [Connection Logs](../monitoring/connection-logs.md): Learn about Connection
  Logs and monitoring.
