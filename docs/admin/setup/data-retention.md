# Data Retention

Coder supports configurable retention policies that automatically purge old
Audit Logs, Connection Logs, and API keys. These policies help manage database
growth by removing records older than a specified duration.

## Overview

Large deployments can accumulate significant amounts of data over time.
Retention policies help you:

- **Reduce database size**: Automatically remove old records to free disk space.
- **Improve performance**: Smaller tables mean faster queries and backups.
- **Meet compliance requirements**: Configure retention periods that align with
  your organization's data retention policies.

> [!NOTE]
> Retention policies are disabled by default (set to `0`) to preserve existing
> behavior. The only exception is API keys, which defaults to 7 days.

## Configuration

You can configure retention policies using CLI flags, environment variables, or
a YAML configuration file.

### Settings

| Setting         | CLI Flag                      | Environment Variable              | Default          | Description                                                              |
|-----------------|-------------------------------|-----------------------------------|------------------|--------------------------------------------------------------------------|
| Global          | `--global-retention`          | `CODER_GLOBAL_RETENTION`          | `0` (disabled)   | Default retention for all data types. Individual settings override this. |
| Audit Logs      | `--audit-logs-retention`      | `CODER_AUDIT_LOGS_RETENTION`      | `0` (use global) | How long to retain Audit Log entries.                                    |
| Connection Logs | `--connection-logs-retention` | `CODER_CONNECTION_LOGS_RETENTION` | `0` (use global) | How long to retain Connection Log entries.                               |
| API Keys        | `--api-keys-retention`        | `CODER_API_KEYS_RETENTION`        | `7d`             | How long to retain expired API keys.                                     |

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
  --global-retention=90d \
  --audit-logs-retention=365d \
  --api-keys-retention=7d
```

### Environment Variables Example

```bash
export CODER_GLOBAL_RETENTION=90d
export CODER_AUDIT_LOGS_RETENTION=365d
export CODER_API_KEYS_RETENTION=7d
```

### YAML Configuration Example

```yaml
retention:
  global: 90d
  audit_logs: 365d
  connection_logs: 0s
  api_keys: 7d
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

For each data type, the effective retention is determined as follows:

1. If the individual setting is non-zero, use that value.
2. If the individual setting is zero, use the global retention value.
3. If both are zero, retention is disabled (data is kept indefinitely).

### API Keys Special Behavior

API key retention only affects **expired** keys. A key is deleted only when:

1. The key has expired (past its `expires_at` timestamp).
2. The key has been expired for longer than the retention period.

Setting `--api-keys-retention=7d` deletes keys that expired more than 7 days
ago. Active keys are never deleted by the retention policy.

Keeping expired keys for a short period allows Coder to return a more helpful
error message when users attempt to use an expired key.

## Best Practices

### Recommended Starting Configuration

For most deployments, we recommend:

```yaml
retention:
  global: 90d
  audit_logs: 365d
  connection_logs: 0s # Use global
  api_keys: 7d
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

## Disabling Retention

Setting a retention value to `0` means "use global retention", not "disable".
To disable all automatic purging, set global to `0` and leave individual
settings at `0`:

```yaml
retention:
  global: 0s
  audit_logs: 0s
  connection_logs: 0s
  api_keys: 0s
```

There is no way to disable retention for a specific data type while global
retention is enabled. If you need to retain one data type longer than others,
set its individual retention to a larger value.

## Monitoring

The purge process logs deletion counts at the `DEBUG` level. To monitor
retention activity, enable debug logging or search your logs for entries
containing the table name (e.g., `audit_logs`, `connection_logs`, `api_keys`).

## Related Documentation

- [Audit Logs](../security/audit-logs.md): Learn about Audit Logs and manual
  purge procedures.
- [Connection Logs](../monitoring/connection-logs.md): Learn about Connection
  Logs and monitoring.
