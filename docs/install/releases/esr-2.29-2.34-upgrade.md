# Upgrading from ESR 2.29 to 2.34

## Guide Overview

Coder provides Extended Support Releases (ESR) biannually. This guide walks
through upgrading from Coder 2.29 ESR to Coder 2.34 ESR. It
summarizes key changes, highlights breaking updates, and provides a recommended
upgrade process.

Read more about the
[ESR release process](./index.md#extended-support-release) and how Coder
supports it.

## What's New in Coder 2.34

### Coder Agents

[Coder Agents](../../ai-coder/agents/index.md) was introduced in v2.32, and is the long-term replacement for
Coder Tasks. Coder Agents is a native AI coding agent that runs entirely within the Coder control plane, managing the agent loop, conversation state, and workspace provisioning in one place. This gives administrators centralized control over model access, credentials, and audit trails across every agent session. Coder Agents was made Beta in v2.33.

Coder Agents includes the following high-level functionality:

- Supports all major LLM providers
- Multi-turn chat
- Automatic workspace provisioning
- MCP server integration, personal skills, and administrator-managed skills
- ACL-based chat sharing across users and groups
- Admin-configurable advisor for planning and architecture guidance
- Plan and subagent explore modes
- Chat debugging
- Virtual desktop

Administrators have the following levers to configure appropriate access to various parts of Coder Agents:

- Template allow lists for agents
- BYOK for users
- Cost controls
- Configurable chat retention
- Automatic chat archiving
- Configurable system instructions
- Observability via AI Gateway, part of Coder's AI Governance Add-On

> [!CAUTION]
> Coder Tasks is officially deprecated in 2.34. It remains supported through the 2.34 ESR support window
> but receives no new features. Coder recommends migrating to Coder Agents
> and the Chats API now. See the [Tasks to Chats migration guide](../../ai-coder/agents/tasks-to-chats-migration.md)
> for API migration details.

### AI Gateway and AI Governance

AI Gateway, previously AI Bridge, matured into a broader governance and
observability layer for AI usage. It now supports:

- [AI Gateway Proxy](../../ai-coder/ai-gateway/ai-gateway-proxy/index.md).
- OpenAI Responses API interception.
- Expanded Copilot and ChatGPT support.
- Custom Bedrock endpoints.
- Structured logs and client/session views.
- Model filtering.
- Multiple providers of the same type.
- [BYOK](../../ai-coder/ai-gateway/auth.md#bring-your-own-key-byok) and
  [key failover](../../ai-coder/ai-gateway/providers.md#key-failover).

[AI Governance](../../ai-coder/ai-governance.md) adds administrative controls
around AI usage:

- License and seat visibility.
- AI session auditing.

These features help administrators understand who is using AI tools, which
providers are being used, and how spend changes over time.

For more information, visit the
[AI Gateway documentation](../../ai-coder/ai-gateway/index.md).

### Agent Firewall

Agent Firewall, previously Agent Boundaries, moved from an early capability into
a stronger governance primitive for AI agents. It can audit and restrict network
access from agent processes, forward machine-readable logs to the control plane,
track usage, and use [landjail mode](../../ai-coder/agent-firewall/landjail.md)
for environments where changing Linux capabilities is not practical.

For more information, visit the
[Agent Firewall documentation](../../ai-coder/agent-firewall/index.md).

### Service Accounts

[Service accounts](../../admin/users/headless-auth.md) are a
[Premium](../../admin/licensing/index.md) feature and now integrate with workspace
sharing, user and workspace filtering, organization membership, and role
assignment.

### Templates, Prebuilds, and User Secrets

Template and workspace operations received several improvements:

- Terraform modules are [cached per template version](../../tutorials/best-practices/speed-up-templates.md)
  to reduce repeated downloads and make workspace starts more deterministic.
- [Prebuild](../../admin/templates/extending-templates/prebuilt-workspaces.md)
  claiming is more durable and idempotent.
- Prebuild presets are validated with dynamic parameter validation.
- [`coder_env`](../../admin/templates/extending-templates/environment-variables.md)
  supports `merge_strategy`.
- [User secrets](../../user-guides/user-secrets.md) can be created, encrypted,
  audited, and injected into workspaces.
- The dashboard warns about active prebuilds when duplicating templates.

These changes reduce operational surprises for template authors, but templates
that assumed a clean Terraform module download on every build should be tested.

### Security and Networking

Coder added several security and networking controls between 2.29 and 2.34:

- OAuth2 external auth providers now support PKCE, and unknown providers default
  to PKCE unless explicitly disabled.
- Secure auth cookies are now enabled automatically when `CODER_ACCESS_URL` uses
  HTTPS.
- AI Gateway Proxy blocks CONNECT tunnels to private or reserved IP ranges, while
  always exempting the Coder access URL.
- Workspace agents can disable reverse and local port forwarding through agent
  flags.
- Authenticated request rate limiting is keyed by user instead of IP address.
- Kubernetes Gateway API `HTTPRoute` is supported as an alternative to Ingress.
- Helm chart probes are more configurable, and Prometheus and pprof addresses can
  be overridden through chart environment values.
- DERP TLS configuration is wired through the CLI, SDK, tailnet, VPN, agent, and
  health checks.

### Operations and Scale

Large deployments should now have improvements in database, logging, and
observability behavior. Coder added the following:

- Configurable PostgreSQL connection pool settings.
- [Retention configuration](../../admin/setup/data-retention.md) for audit logs,
  connection logs, API keys, and workspace agent logs.
- `dbpurge` metrics.
- Support bundle improvements.
- `chatd` metrics.
- Agent first-connection duration metrics.
- A `coder_build_info` metric.

Coder also removed several deprecated Prometheus metrics, so dashboards and
alerts should be reviewed before the upgrade.

Several expensive queries and write paths were optimized, including:

- AI Gateway session listing.
- Audit and connection log counts.
- Connection log batching.
- Provisioner job queue lookups.
- Chat streaming.
- Coordinator peer mapping.

### CLI and Dashboard Enhancements

The CLI and dashboard gained smaller but meaningful workflow improvements:

- `coder create --no-wait` creates a workspace without waiting for startup.
- `coder logs` provides easier access to logs.
- `coder login token` prints the current session token for scripts and automation.
- `coder support bundle` can infer the workspace from the environment.
- `coder groups list -o json` now returns a flat JSON structure.
- The dashboard includes user editing, service account management, group member
  filtering, role selection during user creation, improved accessibility, and
  clearer confirmation flows for destructive actions.

## Changes to be Aware of

The following changes introduced after 2.29 might break workflows, require manual
updates, or change administrator expectations:

| Initial State (2.29 and before)                                                                                 | New State (2.30-2.34)                                                                                                                                    | Change Required                                                                                                                                                                                                                                         |
|-----------------------------------------------------------------------------------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Terraform modules are downloaded during each workspace start.                                                   | Terraform modules are cached and pinned per template version.                                                                                            | Publish a new template version when upstream module changes should apply. Test templates that relied on fresh module downloads. See [speed up templates](../../tutorials/best-practices/speed-up-templates.md).                                         |
| Integrations may use experimental AI Bridge endpoints under `/api/experimental/aibridge/*`.                     | Experimental AI Bridge endpoints were removed after AI Gateway graduated to stable routes.                                                               | Update clients to use `/api/v2/aibridge/*` routes. Review API consumers again because `/api/v2/aibridge/interceptions` is now deprecated in favor of `/api/v2/aibridge/sessions`. See the [AI Gateway API reference](../../reference/api/aigateway.md). |
| Unknown external OAuth providers did not default to PKCE.                                                       | Unknown external OAuth providers now default to PKCE.                                                                                                    | If a provider does not support PKCE, set `CODER_EXTERNAL_AUTH_<N>_PKCE_METHODS=none`. See [external authentication](../../admin/external-auth/index.md).                                                                                                |
| `--secure-auth-cookie` defaulted independently from the access URL.                                             | Secure auth cookies are enabled automatically when `CODER_ACCESS_URL` uses HTTPS.                                                                        | Confirm reverse proxies send the correct scheme headers. To preserve old behavior, explicitly set `CODER_SECURE_AUTH_COOKIE=false`.                                                                                                                     |
| SFTP and SCP connections always landed in `$HOME`.                                                              | SFTP and SCP now respect the workspace agent `dir` setting.                                                                                              | Update scripts that relied on implicit `$HOME` paths. Prefer explicit absolute paths for file transfers.                                                                                                                                                |
| `coder_agent` `dir` attribute accepted any path without warning.                                                | `dir` is deprecated and emits a warning. Non-`$HOME`/`~` values also break [Coder Desktop file sync](../../user-guides/desktop/desktop-connect-sync.md). | Set `dir` to `$HOME` or omit it on `coder_agent` resources. The attribute still works in 2.34 but will be removed in a future release.                                                                                                                  |
| Pre-2.28 Tasks templates might still exist in older deployments.                                                | The pre-2.28 Tasks template format is no longer supported as of 2.30.                                                                                    | Update Tasks templates to use `app_id` instead of the deprecated `sidebar_app` flow. See the [Tasks migration guide](../../ai-coder/tasks-migration.md).                                                                                                |
| Tasks is the primary AI coding workflow.                                                                        | Coder Agents is the long-term replacement, and Tasks is supported through the 2.34 ESR window (into 2026).                                               | Plan migration from the Tasks API to the Chats API and Coder Agents. See [Migrating from the Tasks API to the Chats API](../../ai-coder/agents/tasks-to-chats-migration.md).                                                                            |
| AI Gateway injected MCP tools can be used for tool exposure.                                                    | Injected MCP tools are deprecated.                                                                                                                       | Move new integrations toward Coder Agents MCP server configuration or the MCP server flow. See [AI Gateway MCP](../../ai-coder/ai-gateway/mcp.md) and [MCP servers](../../ai-coder/agents/platform-controls/mcp-servers.md).                            |
| AI Bridge is opt-in via `CODER_AIBRIDGE_ENABLED` (default `false`).                                             | The toggle is renamed to `CODER_AI_GATEWAY_ENABLED` and now defaults to `true`.                                                                          | The in-memory AI Gateway now starts on every deployment. Set `CODER_AI_GATEWAY_ENABLED=false`, or the deprecated `CODER_AIBRIDGE_ENABLED` alias which still works, to keep the old behavior.                                                            |
| AI Gateway providers are configured with `CODER_AIBRIDGE_PROVIDER_*` or `CODER_AI_GATEWAY_PROVIDER_*` env vars. | Provider configuration is stored in the database. Env vars seed the database once on first startup, then are deprecated.                                 | After upgrade, visit `/ai/settings` to verify seeded providers, then remove the env vars. Coderd fails to start if env vars drift from the seeded database row. See [AI Gateway providers](../../ai-coder/ai-gateway/providers.md).                     |
| Regular users can read their own AI Gateway interceptions.                                                      | Only owners and auditors can read AI Gateway interception data.                                                                                          | Update dashboards, scripts, or user workflows that expected self-service interception reads. This intentionally narrows the RBAC surface.                                                                                                               |
| `coder groups list -o json` returns the old command output shape.                                               | `coder groups list -o json` returns a flat structure matching other list commands.                                                                       | Update scripts that parse this command output.                                                                                                                                                                                                          |
| `coder tokens rm` deletes token records by default.                                                             | `coder tokens rm` expires tokens by default and keeps records for auditability.                                                                          | Use `coder tokens rm --delete` only when the token record must be deleted. Update scripts that expect removed tokens to disappear from token history.                                                                                                   |
| Deprecated Prometheus metrics are still emitted.                                                                | Deprecated Prometheus metrics were removed.                                                                                                              | Update dashboards and alerts that use `coderd_api_workspace_latest_build_total` or `coderd_oauth2_external_requests_rate_limit_total`. Use the replacement metrics without the `_total` suffix.                                                         |
| Authenticated rate limits are effectively shared by client IP in some deployments.                              | Authenticated request rate limits are keyed by user.                                                                                                     | Review monitoring and expectations for NATed users or shared proxies. Per-user limits now apply more consistently after API key precheck.                                                                                                               |
| `coder login` can run while `CODER_SESSION_TOKEN` is set.                                                       | `coder login` errors when `CODER_SESSION_TOKEN` is set.                                                                                                  | Unset `CODER_SESSION_TOKEN` in interactive login flows. Keep using the environment variable for non-interactive automation.                                                                                                                             |
| Workspace starts with new parameters can proceed without an explicit stop in some flows.                        | Workspace starts with new parameters stop the workspace before starting.                                                                                 | Expect downtime when applying new parameters. Update automation that assumes the workspace remains running.                                                                                                                                             |
| `mode=auto` workspace links can silently create workspaces with prefilled parameters.                           | Users must confirm workspace auto-creation before provisioning starts.                                                                                   | Update Open in Coder buttons, runbooks, or internal flows that expect one-click workspace creation without a consent dialog.                                                                                                                            |
| Users with `--login-type none` are common for automation.                                                       | `--login-type none` is deprecated.                                                                                                                       | For Premium deployments, migrate automation to service accounts. For OSS deployments, use regular users with password, GitHub, or OIDC authentication. See [headless auth](../../admin/users/headless-auth.md).                                         |
| Terminal commands can be executed from URL parameters without extra confirmation.                               | The dashboard requires confirmation before executing terminal commands from URLs.                                                                        | Update runbooks or deep links that expected immediate terminal execution. This protects users from accidental command execution.                                                                                                                        |
| Agent SSH port forwarding is always available when the agent allows SSH.                                        | Reverse and local port forwarding can be disabled per agent.                                                                                             | Review templates and IDE workflows before enabling `--block-reverse-port-forwarding` or `--block-local-port-forwarding`. See [port forwarding](../../admin/networking/port-forwarding.md).                                                              |
| `PATCH /api/v2/templates/{template}` accepts value fields for metadata updates.                                 | Template metadata update fields are optional pointer fields in the SDK, and 304 responses were removed.                                                  | Update SDK consumers and direct API clients that patch template metadata. Send only fields that should change, including false or zero values explicitly.                                                                                               |
| External provisioner daemons use the 2.29 provisionerd protocol.                                                | The provisionerd protocol changed for provisioner operations and file upload/download.                                                                   | Update external provisioner daemons to the matching 2.34 protocol. The protocol reserves removed fields such as `stop_modules`, `exp_reuse_terraform_workspace`, and `user_secrets`, and adds `DownloadFile`.                                           |
| Helm chart health probes and observability bind addresses use older chart defaults.                             | Readiness and liveness probes have `enabled` toggles and more fields, and Prometheus/pprof addresses are overridable.                                    | Review custom Helm values for probe behavior and observability bindings. Prefer restricting pprof to a local address when exposing diagnostics.                                                                                                         |

## Upgrading

> [!NOTE]
> You can upgrade directly from 2.29 to 2.34. Stepping through intermediate
> minor versions is not required.
>
> This upgrade applies 108 database migrations. Coder applies them in order
> on startup. Most are fast schema changes, but a few rewrite or backfill
> long-lived tables and hold locks while they run. Total time ranges from under
> a minute to several minutes, scaling with the size of the tables called out
> in [Database migrations to watch](#database-migrations-to-watch) below.
>
> Take a database backup before upgrading and validate the upgrade in a
> staging environment that mirrors production data volume.

### Database migrations to watch

The batch runs in order on the first startup of the new version. Most
migrations create new tables or make fast schema changes, but the following
pre-existing tables receive the heaviest operations. Size your maintenance
window for whichever are largest in your deployment:

- **Tailnet coordination tables** (`tailnet_peers`, `tailnet_tunnels`,
  `tailnet_coordinators`) are converted to `UNLOGGED` and rewritten under an
  exclusive lock. **`UNLOGGED` tables are not replicated to standby servers and
  are truncated on crash recovery.** This is intentional, since coordinators
  re-register and peers reconnect on startup, but confirm your high
  availability strategy does not rely on replicating tailnet state to read
  replicas.
- **`users`** gains a service account column plus check constraints and unique
  index rebuilds, held under an exclusive lock. This briefly blocks logins and
  API key validation, so the duration matters most on deployments with many
  users.
- **`workspace_agents`** (joined with `workspace_builds`, `workspace_resources`,
  and `workspaces`) is bulk updated to soft-delete stale agents left behind by
  a pre-2.33 bug. This is typically the slowest step on long-lived deployments
  with extensive build history. It is safe, but plan for the time.
- **`workspaces`** receives full-table updates and new ACL check constraints.
- **`usage_events`** has a check constraint revalidated and an index added; the
  cost scales with retained event volume.

Several of these changes are irreversible, including the `users` service
account reclassification and the cleanup of `user_secrets`,
`organization_members`, and related rows for already soft-deleted users. Take a
database backup before upgrading.

The Coder team recommends taking the following steps when performing the upgrade:

- **Perform the upgrade in a staging environment first:** The cumulative changes
  between 2.29 and 2.34 affect AI workflows, templates, prebuilds,
  authentication, RBAC, and dashboard behavior. Validate representative
  workspaces before production rollout.
- **Retest templates and prebuilds:** Focus on Terraform module caching,
  prebuild preset validation, `coder_env` merging, user secrets, and workspace
  starts with changed parameters.
- **Audit AI Gateway integrations:** Update experimental API routes, check
  permissions for interception/session data, migrate provider configuration
  from env vars to the database via `/ai/settings`, verify proxy mode behavior,
  and review any injected MCP usage.
- **Plan the Tasks to Agents migration:** Tasks remains available during the
  support window, but new automation should use Coder Agents and the Chats API.
  Update internal docs, templates, and API clients accordingly.
- **Validate external authentication:** Test GitHub, GitLab, OIDC, and custom
  external auth providers. Disable PKCE for providers that do not support it.
- **Migrate headless automation to service accounts:** Replace users created
  with `--login-type none` where possible, and verify CI/CD tokens, template
  publish jobs, and workspace automation.
- **Update CLI parsers, API clients, and scripts:** Check `coder groups list -o
  json`, `coder tokens rm`, `coder login` with `CODER_SESSION_TOKEN`, SFTP/SCP
  destination paths, template metadata update clients, provisionerd protocol
  consumers, and any script that depends on terminal command URL execution.
- **Review networking controls before enabling them:** Test AI Gateway Proxy,
  private IP restrictions, port forwarding blocks, DERP TLS configuration,
  Kubernetes `HTTPRoute`, and Helm probe settings in environments that use custom
  networking.
- **Tune operational settings after rollout:** Review PostgreSQL connection pool
  settings, retention policies, dbpurge behavior, Prometheus metrics, secure
  cookie behavior, support bundle output, and log ingestion pipelines.
- **Communicate user-facing changes:** Service accounts, Coder Agents, AI
  Governance, Tasks deprecation, dashboard confirmations, and workspace parameter
  restarts can change user workflows. Share the expected behavior before the
  production upgrade.
