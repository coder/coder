# Upgrading from ESR 2.24 to 2.29

## Overview

This guide walks through upgrading from the initial Coder 2.24 ESR release to our new 2.29 ESR release. It will summarize key changes, highlight breaking updates, and provide a recommended upgrade process. 

## What's New in Coder 2.29

### Coder Tasks

Beginning in Coder 2.24, Tasks were introduced as an experimental feature that allowed administrators and developers to run long-lived or automated operations from templates. Over the subsequent releases, Tasks matured significantly through UI refinement, improved reliability, and underlying task-status improvements in the server and database layers. By 2.29, Tasks were formally promoted to general availability, with full CLI support, task-specific UI displays (such as icons, display names, and startup-script error alerts), and consistent visibility of task states across the dashboard. This transition establishes Tasks as a stable automation and job-execution primitive within Coder. For more information, read our documentation [here](https://coder.com/docs/ai-coder/tasks). 

### AI Bridge

Across releases 2.26 through 2.29, AI Bridge evolved from a limited preview into a production-ready system. Coder added a dedicated page for interception logs, introduced request-duration visibility for observability, implemented configurable log-retention settings, and exposed AI Bridge metrics for operational monitoring. The documentation was expanded with an AI Bridge client configuration guide, improved key-scopes documentation for OpenAI/Anthropic providers, and setup instructions for Amazon Bedrock. Terminology was standardized to "AI Bridge" across the product. For more information, read our documentation [here](https://coder.com/docs/ai-coder/ai-bridge).

### AI Boundaries

From 2.24 to 2.29, agent boundaries themselves did not change, but the systems that enforce them became much more reliable. The introduction of the Agent Unit Manager, the Agent Socket API, and improvements to agent readiness, permission checks, and authorization handling make boundary evaluation more consistent and predictable. These upgrades strengthen how agents interact with workspaces and reduce the chances of unexpected agent behavior, resulting in a more stable and trustworthy boundary model for AI-driven workflows.

### Performance Enhancements

Performance, particularly at scale, improved across nearly every system layer. Database queries were optimized, several new indexes were added, and expensive migrations—such as migration 371—were reworked to complete faster on large deployments. Caching was introduced for Terraform installer files and workspace/agent lookups, reducing repeated calls. Notification performance improved through more efficient connection pooling. These changes collectively enable deployments with hundreds or thousands of workspaces to operate more smoothly and with lower resource contention.

### Server and API Updates

Core server capabilities expanded significantly across the releases. Prebuild workflows gained timestamp-driven invalidation via last_invalidated_at, expired API keys began being automatically purged, and new API key-scope documentation was introduced to help administrators understand authorization boundaries. New API endpoints were added, including the ability to modify a task prompt or look up tasks by name. Template developers benefited from new Terraform directory-persistence capabilities (opt-in on a per-template basis) and improved `protobuf` configuration metadata.

### CLI Enhancements

The CLI gained substantial improvements between the two versions. Most notably, beginning in 2.29, Coder’s CLI now stores session tokens in the operating system keyring by default on macOS and Windows, enhancing credential security and reducing exposure from plaintext token storage. Users who rely on directly accessing the token file can opt out using `--use-keyring=false`. The CLI also introduced cross-platform support for keyring storage, gained support for GA Task commands, and integrated experimental functionality for the new Agent Socket API.

## Changes to be Aware of

The following are changes introduced after 2.24.X that might break workflows, or require other manual effort to address:

- **Workspace updates now forcibly stop workspaces before updating**: 
Starting in 2.24.x, updates no longer occur in place. Anyone relying on seamless updates or scripted update flows will see interruptions, as users must now expect downtime during updates
- **Connection events moved out of the Audit Log and older audit entries were pruned:** Beginning in 2.25, SSH/port-forward/browser-connection events are logged only in the Connection Log, and historical connection events older than 90 days were removed. This affects teams with compliance, audit, or ingestion pipelines that rely on older audit-log formats
- **:CLI session tokens are now stored in the OS keyring on macOS and Windows:** This begins in 2.29. Any scripts, automation, or SSO flows that directly read or modify the old plaintext token file will break unless users disable keyring usage via `--use-keyring=false`
- **`task_app_id` was removed from codersdk.WorkspaceBuild:** Integrations or custom tools built against the 2.24 API that reference this field must migrate to Task.WorkspaceAppID. The old field no longer returns data and may break deserialization or lookups
- **OIDC refresh-token behavior is more strict compared to 2.24:** Misconfigured OIDC providers may cause forced re-authentication or failed CLI logins after upgrading. Administrators should ensure refresh tokens are enabled and configured per the updated documentation
- **Template behavior changed for devcontainers with multiple agents:** The devcontainer agent selection is no longer random and now requires explicit choice. Automated workflows that assumed the previous implicit behavior will need updating
- **Terraform workflows now use persistent or cached directories when enabled:** Introduced after 2.24, these features can impact template authors who rely on clean Terraform execution directories or per-build isolation
- **Several agent and task lifecycle behaviors have become more strict:** Although not breaking in isolation, workflows built on 2.24’s more permissive or inconsistent agent/task readiness may encounter stricter permission checks, readiness gating, or ordering differences after upgrading

## Upgrading

The following are recommendations by the Coder team when performing the upgrade:

- **Perform the upgrade in a staging environment first:** The cumulative changes between 2.24 and 2.29 introduce new subsystems and lifecycle behaviors, so validating templates, authentication flows, and workspace operations in staging helps avoid production issues
- **Audit scripts or tools that rely on the CLI token file:** Since 2.29 uses the OS keyring for session tokens on macOS and Windows, update any tooling that reads the plaintext token file or plan to use `--use-keyring=false`
- **Review templates using devcontainers or Terraform:** Explicit agent selection, optional persistent/cached Terraform directories, and updated metadata handling mean template authors should retest builds and startup behavior
- **Check and update OIDC provider configuration:** Stricter refresh-token requirements in later releases can cause unexpected logouts or failed CLI authentication if providers are not configured according to updated docs
- **Update integrations referencing deprecated API fields:** Code relying on `WorkspaceBuild.task_app_id` must migrate to `Task.WorkspaceAppID`, and any custom integrations built against 2.24 APIs should be validated against the new SDK
- **Communicate audit-logging changes to security/compliance teams:** From 2.25 onward, connection events moved into the Connection Log, and older audit entries may be pruned, which can affect SIEM pipelines or compliance workflows
- **Validate workspace lifecycle automation:** Since updates now require stopping the workspace first, confirm that automated update jobs, scripts, or scheduled tasks still function correctly in this new model
- **Retest agent and task automation built on early experimental features:** Updates to agent readiness, permission checks, and lifecycle ordering may affect workflows developed against 2.24’s looser behaviors
- **Monitor workspace, template, and Terraform build performance:** New caching, indexes, and DB optimizations may change build times; observing performance post-upgrade helps catch regressions early
- **Prepare user communications around Tasks and UI changes:** Tasks are now GA and more visible in the dashboard, and many UI improvements will be new to users coming from 2.24, so a brief internal announcement can smooth the transition
