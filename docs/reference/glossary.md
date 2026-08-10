# Glossary

This glossary defines the Coder-specific terms and product names you encounter across the documentation.
Each entry gives a short definition and, where it helps, links to the page that covers the term in depth.

> [!IMPORTANT]
> Several Coder terms share the word "agent" but mean different things:
>
> - [Coder Agents](#coder-agents) is the AI product for delegating development work to coding agents.
> - A [workspace agent](#workspace-agent) is the process that runs inside a workspace to provide SSH, port forwarding, the web terminal, and other services.
> - [`coder_agent`](#coder_agent) is the Terraform resource in a template that declares a workspace agent.

## A

### Agent API

The API a workspace agent uses to report logs, metrics, and activity.
Refer to the [Agent API reference](./agent-api/index.md).

### Agent Firewall

A process-level firewall that enforces domain and verb allowlists on AI agent processes inside a workspace and streams audit logs to `coderd`.
It was previously named Agent Boundaries and uses a sandbox backend, `nsjail` by default or `landjail`.
This feature requires the AI Governance Add-On.
Refer to [Agent Firewall](../ai-coder/agent-firewall/index.md).

### Agent Workspace Build

A metered workspace build performed on behalf of an AI agent.
Community and Premium deployments include 1,000 for proof-of-concept use, and the AI Governance Add-On expands the allowance.
Refer to [AI Governance](../ai-coder/ai-governance.md).

### AI Gateway

An LLM gateway in `coderd` that authenticates users, forwards traffic to providers such as OpenAI and Anthropic, audits prompts and tool invocations, and centralizes MCP administration.
It was previously named AI Bridge and runs the `aibridged` component in memory inside `coderd`.
This feature requires the AI Governance Add-On.
Refer to [AI Gateway](../ai-coder/ai-gateway/index.md).

### AI Gateway Proxy

An HTTP proxy component, `aibridgeproxyd`, for AI clients that cannot override their base URL, such as GitHub Copilot.
This feature requires the AI Governance Add-On.
Refer to [AI Gateway Proxy](../ai-coder/ai-gateway/ai-gateway-proxy/index.md).

### AI Governance Add-On

A separate per-user license for Premium customers, purchased on top of a Premium subscription, that unlocks AI Gateway and Agent Firewall and expands Agent Workspace Build allowances.
Refer to [AI Governance](../ai-coder/ai-governance.md).

### Air-gapped deployment

A Coder installation that has no outbound internet access.
Refer to [Air-gapped deployments](../install/airgap.md).

### Audit logging

Structured logs of user and admin operations.
This is a Premium feature.
Refer to [Audit logs](../admin/security/audit-logs.md).

### Autostop requirement

A template or deployment setting that forcibly stops workspaces on a recurring cadence, for example to apply updates.
Refer to [Workspace scheduling](../admin/templates/managing-templates/schedule.md).

## B

### Built-in roles

The predefined roles, including User Admin, Template Admin, Auditor, and Member, with progressively narrower permissions.
Refer to [Groups and roles](../admin/users/groups-roles.md).

## C

### CDE

Cloud development environment, the category of product that hosts development environments on remote infrastructure.
A Coder workspace is a CDE.

### Change management

The GitOps flow that stores templates in Git and pushes them through CI/CD with `coder templates push`.
Refer to [Change management](../admin/templates/managing-templates/change-management.md).

### code-server

The Coder-maintained build of VS Code that runs in the browser.
Refer to [code-server](../user-guides/workspace-access/code-server.md).

### Coder

The self-hosted platform that provisions and manages remote development environments, called workspaces, as code.
`Coder`, the company and the product, is always capitalized.

### Coder Agents

The native, self-hosted AI product for delegating development work and research to coding agents.
The agent loop runs in the control plane, and developers work through the dashboard, the `coder agents` CLI, or the REST API.
Not to be confused with a [workspace agent](#workspace-agent) or the [`coder_agent`](#coder_agent) resource.
Refer to [Coder Agents](../ai-coder/agents/index.md).

### Coder Agents User

The per-organization role that a member needs to use [Coder Agents](#coder-agents).
Refer to [Coder Agents](../ai-coder/agents/index.md).

### Coder CLI

The single `coder` binary used for admin and user operations.
The same binary runs inside a workspace as the workspace agent through `coder agent`.
Refer to the [CLI reference](./cli/index.md).

### Coder Connect

The Coder Desktop feature that runs a local tunnel so workspaces are reachable at `workspace-name.coder` hostnames.
Refer to [Coder Desktop](../user-guides/desktop/index.md).

### Coder Desktop

A native desktop application that connects your machine to your workspaces so you can reach them by hostname and launch apps without configuring the CLI.
Refer to [Coder Desktop](../user-guides/desktop/index.md).

### Coder extension for VS Code

The editor extension that connects VS Code, and forks such as Cursor and Windsurf, to Coder workspaces.
Refer to [VS Code](../user-guides/workspace-access/vscode.md).

### Coder Tasks

An earlier interface for running coding agents such as Claude Code and Aider inside workspaces.
Coder Tasks is deprecated: it moves to a 12-month Extended Support Release for Premium customers and is removed from new releases starting with v2.37, with [Coder Agents](#coder-agents) as the long-term replacement.
Refer to [Coder Tasks](../ai-coder/tasks.md).

### `coder_agent`

The Terraform resource that declares a [workspace agent](#workspace-agent) inside a template.
Refer to the [`coder_agent` resource](https://registry.terraform.io/providers/coder/coder/latest/docs/resources/agent).

### `coder_app`

The Terraform resource that declares an app, IDE, or URL entry surfaced on the workspace page, such as VS Code Web, JupyterLab, or code-server.
Refer to the [`coder_app` resource](https://registry.terraform.io/providers/coder/coder/latest/docs/resources/app).

### `coder_metadata`

The Terraform resource that surfaces static resource information in the dashboard.
Refer to the [`coder_metadata` resource](https://registry.terraform.io/providers/coder/coder/latest/docs/resources/metadata).

### `coder_script`

The Terraform resource that runs a shell script inside the workspace on start or stop, or on demand.
Refer to the [`coder_script` resource](https://registry.terraform.io/providers/coder/coder/latest/docs/resources/script).

### `coderd`

The Coder control plane server, started with `coder server`.
It serves the dashboard and API, brokers connections to workspaces, and stores state in PostgreSQL.
Refer to [Architecture](../admin/infrastructure/architecture.md).

### codersdk

The Go SDK that the CLI and dashboard use and that you can use for automation.
Refer to the [`codersdk` package](https://pkg.go.dev/github.com/coder/coder/v2/codersdk).

### Coding agent

An AI agent that reads and writes code on a developer's behalf, such as Claude Code, run through Coder Tasks or Coder Agents.
Refer to [AI in Coder](../ai-coder/index.md).

### Community

The free, open-source tier of Coder.

### Computational resource

A resource that provides compute for the workspace, such as a virtual machine or container, and typically runs the workspace agent.

### Connection logs

Structured logs of connections to workspaces, such as SSH and application sessions.
This is a Premium feature.
Refer to [Connection logs](../admin/monitoring/connection-logs.md).

### Control plane

The collective term for `coderd`, its provisioners, and its database.
The control plane also runs the agent loop for [Coder Agents](#coder-agents).

### Custom agent

A coding agent you integrate with Coder yourself, beyond the built-in options.
Refer to [Custom agents](../ai-coder/custom-agents.md).

### Custom roles

Deployment-defined roles composed of specific RBAC actions.
This is a Premium feature.
Refer to [Groups and roles](../admin/users/groups-roles.md).

## D

### Deployment

A single installation of Coder, comprising the control plane, its provisioners, and its database.

### Deployment ID

The unique identifier for a Coder deployment.
You provide it when you request a trial or a license.

### DERP

Designated Encrypted Relay for Packets.
The Tailnet uses DERP relays to carry traffic when a direct peer-to-peer connection between two nodes is not possible.
Refer to [Networking](../admin/networking/index.md).

### Dev container

An environment that conforms to the [Development Container Specification](https://containers.dev/) and is defined by a `devcontainer.json` file.
`Dev Containers` is the Coder feature that runs dev containers inside a workspace.
Refer to [Dev containers](../admin/templates/extending-templates/devcontainers.md).

### `display_apps`

A field on `coder_agent` that toggles the built-in agent apps, such as `vscode`, `vscode_insiders`, `web_terminal`, `ssh_helper`, and `port_forwarding_helper`.
The enabled apps appear as buttons in the row on the workspace page.
Refer to [Web IDEs](../admin/templates/extending-templates/web-ides.md).

### Dormancy

Automatic cleanup of workspaces that stay inactive past a threshold, up to and including deletion.
This is a Premium feature.
Refer to [Workspace scheduling](../admin/templates/managing-templates/schedule.md).

### Dotfiles

Personal shell and editor configuration loaded into a workspace with `coder dotfiles`.
Refer to [Dotfiles](../user-guides/workspace-dotfiles.md).

### Dynamic parameters

Template inputs that can change per build, with conditional visibility and validation.
Refer to [Dynamic parameters](../admin/templates/extending-templates/dynamic-parameters.md).

## E

### Envbuilder

The Coder-built tool that constructs a workspace image from a `devcontainer.json` file without requiring Docker inside the workspace.
Refer to [Envbuilder](../admin/integrations/devcontainers/envbuilder/index.md) and the [`envbuilder` repository](https://github.com/coder/envbuilder).

### Ephemeral resource

A resource that is destroyed when the workspace stops and recreated on the next start.
Refer to [Resource persistence](../admin/templates/extending-templates/resource-persistence.md).

### External authentication

In-workspace OAuth to Git providers, artifact registries, and similar services, configured with `CODER_EXTERNAL_AUTH_*` variables.
Refer to [External authentication](../admin/external-auth/index.md).

### External provisioner

A `provisionerd` that runs outside `coderd`, tagged so specific templates route to it, for example to reach an isolated network or to scale build throughput.
Refer to [External provisioners](../admin/provisioners/index.md).

### External workspace

A workspace that runs on infrastructure Coder did not provision, connected to Coder through an agent.
This is a Premium feature in early access.
Refer to [External workspaces](../admin/templates/managing-templates/external-workspaces.md).

## F

### Feature stages

The Early Access, Beta, and General Availability labels that describe how production-ready a feature is.
Refer to [Feature stages](../install/releases/feature-stages.md).

## G

### Group

A collection of users used to grant template and workspace permissions and to apply quotas.
Refer to [Groups and roles](../admin/users/groups-roles.md).

## H

### Headless authentication

Authentication for automated users and service accounts that cannot complete an interactive browser login.
Refer to [Headless authentication](../admin/users/headless-auth.md).

### High availability

Running multiple `coderd` replicas behind a load balancer for redundancy and scale.
This is a Premium feature.
Refer to [High availability](../admin/networking/high-availability.md).

## I

### IDE agent

A coding agent embedded in an editor or IDE, such as Cursor, connected to a Coder workspace.
Refer to [IDE agents](../ai-coder/ide-agents.md).

### IdP sync

Automatic mapping of identity-provider claims to Coder groups, roles, and organizations.
This is a Premium feature.
Refer to [IdP sync](../admin/users/idp-sync.md).

### Infrastructure as code

The practice of defining infrastructure in version-controlled configuration files.
Coder templates are infrastructure as code, written in Terraform.

## L

### License key

A signed token applied through the dashboard or with `coder licenses add`.
Coder validates the key locally, so it works in air-gapped deployments.
Refer to [Licensing](../admin/licensing/index.md).

## M

### MCP

Model Context Protocol, the standard for exposing tools and servers to LLM agents.
Coder ships an MCP server and lets admins centrally manage approved MCP servers through AI Gateway.
Refer to [MCP server](../ai-coder/mcp-server.md).

### Module

A reusable Terraform building block that a template composes in.
Coder publishes modules in the [Coder Registry](https://registry.coder.com/).
Refer to [Modules](../admin/templates/extending-templates/modules.md).

## O

### OIDC

OpenID Connect.
Coder supports any specification-compliant OIDC provider for single sign-on.
Refer to [OIDC authentication](../admin/users/oidc-auth/index.md).

### OpenTofu

An open-source alternative to Terraform.
Coder deployments can run a custom Terraform binary such as OpenTofu, though custom Terraform binaries are not officially supported.
Refer to [Provisioning with OpenTofu](../admin/integrations/opentofu.md).

### Organization

An isolation boundary for members, templates, provisioners, and quotas.
Multi-organization support is a Premium feature.
Refer to [Organizations](../admin/users/organizations.md).

### Owner

The top-level administrator role, with full access to the deployment and every organization.
Refer to [Groups and roles](../admin/users/groups-roles.md).

## P

### Peripheral resource

A supporting resource, such as a disk or network object, that a computational resource depends on.

### Persistent resource

A resource that stays provisioned when the workspace stops, so its state survives a restart.
Refer to [Resource persistence](../admin/templates/extending-templates/resource-persistence.md).

### Personal access token

An API token, often abbreviated PAT, that a user creates to authenticate CLI and API requests.
Refer to [Sessions and tokens](../admin/users/sessions-tokens.md).

### Port forwarding

Access to arbitrary workspace ports through the CLI, SSH, or subdomain apps.
Refer to [Port forwarding](../user-guides/workspace-access/port-forwarding.md).

### Prebuilt workspaces

A pool of workspaces provisioned ahead of time and ready to claim, which cuts the time to first launch.
This is a Premium feature.
Refer to [Prebuilt workspaces](../admin/templates/extending-templates/prebuilt-workspaces.md).

### Premium

The paid enterprise tier, which unlocks features such as multi-organization support, high availability, workspace proxies, audit logs, quotas, custom roles, SCIM, and IdP sync.
Visit [Pricing](https://coder.com/pricing).

### Prometheus metrics

Deployment metrics exposed on a `/metrics` endpoint for Prometheus to scrape.
Refer to [Prometheus](../admin/integrations/prometheus.md).

### Provisioner

A `provisionerd` instance that executes template builds.
Refer to [External provisioners](../admin/provisioners/index.md).

### Provisioner tags

Key-value tags on templates and provisioner daemons that route a build to a matching provisioner.
Refer to [External provisioners](../admin/provisioners/index.md).

### `provisionerd`

The daemon that runs Terraform to create, update, and destroy workspace resources.
It runs bundled with `coderd` by default and can also run externally.
Refer to [External provisioners](../admin/provisioners/index.md).

## Q

### Quiet hours

A window during which forced autostops do not run, so they do not interrupt a developer who is working.
Refer to [Workspace scheduling](../user-guides/workspace-scheduling.md).

## R

### RBAC

Role-based access control, the model Coder uses to grant permissions through roles.
Refer to [Groups and roles](../admin/users/groups-roles.md).

### Registry

The [Coder Registry](https://registry.coder.com/), where Coder publishes reusable templates and modules.

### Release channels

Coder's supported release lines: mainline, stable, and Extended Support Release.
Refer to [Releases](../install/releases/index.md).

### Resource

Any Terraform resource created by a template build, whether or not it runs a workspace agent.
Refer to [Resource persistence](../admin/templates/extending-templates/resource-persistence.md).

### Resource quota

A per-user or per-organization cap on the cost of workspace resources.
This is a Premium feature.
Refer to [Quotas](../admin/users/quotas.md).

### REST API

Coder's HTTP API, served under `/api/v2/*`.
Refer to the [API reference](./api/index.md).

## S

### SCIM

Automated user provisioning and deprovisioning driven by your identity provider.
This is a Premium feature.
Refer to [IdP sync](../admin/users/idp-sync.md).

### Service account

A non-human user intended for automation.
You can filter for one in the users list with the `service_account:true` query.
Refer to [Users](../admin/users/index.md).

### Sessions and tokens

Session cookies authenticate the web dashboard, and API tokens authenticate programmatic access.
Refer to [Sessions and tokens](../admin/users/sessions-tokens.md).

### SSH

Optimized SSH access to a workspace over Coder's Tailnet routing, available through `coder ssh`.
Refer to [Workspace access](../user-guides/workspace-access/index.md).

### SSO

Single sign-on, the pattern of authenticating to Coder through an external identity provider, typically over OIDC.
Refer to [OIDC authentication](../admin/users/oidc-auth/index.md).

### Starter template

A curated example template that ships with Coder for a common platform, such as Docker, Kubernetes, AWS, GCP, or Azure.
Refer to [Create templates](../admin/templates/creating-templates.md).

### Sub-agent (Coder Agents)

A delegated child chat that a parent Coder Agents chat spawns to perform a sub-task, with its own tool allowlist.
Not to be confused with a [dev container sub-agent](#sub-agent-dev-containers).
Refer to [Coder Agents architecture](../ai-coder/agents/architecture.md).

### Sub-agent (dev containers)

The nested workspace agent that Coder runs for a dev container inside a parent workspace, so you can connect to the dev container directly.
This is a Coder term, not part of the Development Container Specification.
Not to be confused with a [Coder Agents sub-agent](#sub-agent-coder-agents).
Refer to [Dev containers](../admin/integrations/devcontainers/integration.md).

### Subdomain apps

Wildcard-hosted URLs that expose a workspace's HTTP services on their own subdomain.
Refer to [Port forwarding](../user-guides/workspace-access/port-forwarding.md).

### Support bundle

A diagnostic archive generated with `coder support bundle` to help debug a deployment.
Refer to [Support bundle](../support/support-bundle.md).

### Supported editors and IDEs

The editors and IDEs that connect to Coder workspaces, including [VS Code](../user-guides/workspace-access/vscode.md), [code-server](../user-guides/workspace-access/code-server.md), [Cursor](../user-guides/workspace-access/cursor.md), [Windsurf](../user-guides/workspace-access/windsurf.md), [Antigravity](../user-guides/workspace-access/antigravity.md), [Zed](../user-guides/workspace-access/zed.md), and JetBrains IDEs through [Gateway](../user-guides/workspace-access/jetbrains/gateway.md) and [Fleet](../user-guides/workspace-access/jetbrains/fleet.md).

### Swagger

An optional OpenAPI endpoint that `coderd` can expose for the REST API.
Refer to the [API reference](./api/index.md).

## T

### Tailnet

The WireGuard-based mesh that connects the control plane, workspace proxies, and workspaces.
Refer to [Networking](../admin/networking/index.md).

### Template

The Terraform configuration that defines the infrastructure and Coder resources, such as the agent, apps, and parameters, that produce a workspace.
Refer to [Templates](../admin/templates/index.md).

### Template hardening

Terraform patterns, such as `prevent_destroy` and `ignore_changes`, that protect persistent resources from accidental destruction.
Refer to [Resource persistence](../admin/templates/extending-templates/resource-persistence.md).

### Template usage insights

Dashboard views and `GET /api/v2/insights/*` endpoints that report active users, app usage, and parameter usage for templates.
Refer to the [Insights API](./api/insights.md).

### Template version

An immutable snapshot of a template.
Workspaces are pinned to a version, and admins can push new versions.
Refer to [Manage templates](../admin/templates/managing-templates/index.md).

### Terraform

The HashiCorp tool Coder uses to define and provision workspace infrastructure.
Visit the [Terraform documentation](https://developer.hashicorp.com/terraform).

## W

### Web terminal

A browser terminal served by the workspace agent.
Refer to [Web terminal](../user-guides/workspace-access/web-terminal.md).

### Workspace

A developer's on-demand development environment, such as a virtual machine, container, or Kubernetes pod, provisioned from a template.
Refer to [Workspace management](../user-guides/workspace-management.md).

### Workspace agent

The `coder agent` process that runs inside a workspace to provide SSH, port forwarding, the web terminal, IDE connections, liveness checks, metadata, logs, and startup scripts.
A template declares it with the [`coder_agent`](#coder_agent) Terraform resource.
Not to be confused with [Coder Agents](#coder-agents), the AI product.

### Workspace app

An application, IDE, or URL surfaced on the workspace page, declared with the [`coder_app`](#coder_app) resource or enabled through [`display_apps`](#display_apps).
Refer to [Web IDEs](../user-guides/workspace-access/web-ides.md).

### Workspace autostart and autostop

Scheduled policies that start a workspace on a set schedule and stop it after a period of inactivity.
Refer to [Workspace scheduling](../user-guides/workspace-scheduling.md).

### Workspace process logging

A record of the processes executed inside workspaces.
This is a Premium feature.
Refer to [Process logging](../admin/templates/extending-templates/process-logging.md).

### Workspace proxy

A regional proxy that terminates user connections closer to the developer to lower latency.
This is a Premium feature.
Refer to [Workspace proxies](../admin/networking/workspace-proxies.md).

## Learn more

- [Architecture](../admin/infrastructure/architecture.md)
- [Coder Agents](../ai-coder/agents/index.md)
- [Templates](../admin/templates/index.md)
- [API reference](./api/index.md)
