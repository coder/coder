# Validate your deployment

Validation proves the deployment works and holds up under load.
Harden the deployment first, then measure it, so that the numbers reflect the configuration you will actually run.

## Harden first

- Confirm the deployment reports healthy in [Health check](../../admin/monitoring/health-check.md).
- Turn on the telemetry you need to interpret a load test, including [metrics](../../admin/monitoring/metrics.md) and [logs](../../admin/monitoring/logs.md).
- Apply the controls a security reviewer expects, such as [audit logs](../../admin/security/audit-logs.md) and [database encryption](../../admin/security/database-encryption.md).

## Then measure

- Run a load test that reflects your expected usage with [Scale testing](./scale-testing.md).
- Compare the results against the size target you chose in [Plan your deployment](../plan/index.md).
  A result that misses the target is a planning question, not a testing failure.

## Before you move on

You should have a template developers can build a workspace from, a successful workspace build by someone other than the person who installed Coder, and a load test result you're willing to show the team that owns the infrastructure.

Next: [Operate and maintain](../operate/index.md).

<children></children>
