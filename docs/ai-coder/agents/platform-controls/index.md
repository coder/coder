# Platform Controls

## Design philosophy

Coder Agents is built on a simple premise: platform teams should have full
control over how agents operate, and developers should have zero configuration
burden.

This means:

- **All agent configuration is admin-level.** Providers, models, system prompts, and tool permissions are set by administrators from the control plane.
  These are not user preferences.
  Deployment admins own the deployment-wide policies, and organization admins own the models and MCP servers of their organization.
  Refer to [Organization scope](./organizations.md) for the split between the 2 scopes.
- **Developers never need to configure anything by default.** A developer just
  describes the work they want done. They do not need to pick a provider or
  write a system prompt. The platform team has already set all of that up.
  When a platform team enables user API keys for a provider, developers may
  optionally supply their own key, but this is an opt-in policy decision, not
  a requirement.
- **Enforcement, not defaults.** Settings configured by administrators are enforced server-side.
  Developers cannot override them, except through the personal model overrides that an administrator turns on.
  A setting that a user can change is a preference, not a policy.

This is an architectural decision, not just a product choice. Because the agent
loop runs in the control plane rather than inside developer workspaces, there is
no local configuration for developers to modify and no agent software for them
to reconfigure. The control plane is the single source of truth for how agents
behave.

## What platform teams control today

### Providers and models

Administrators configure which LLM providers and models are available from the
Coder dashboard. This includes API keys, base URLs (for enterprise proxies or
self-hosted models), and per-model parameters like context limits, thinking
budgets, and reasoning effort.

Providers are deployment-wide.
Models belong to an organization, so each organization has its own model list and its own default model.

Developers select from the set of models an administrator has enabled. They
cannot add their own providers or access models that have not been explicitly
configured.

When an administrator enables user API keys on a provider, developers can
supply their own key from the Agents settings page. Refer to
[User API keys (BYOK)](../models.md#user-api-keys-byok) for details.

Refer to [Models](../models.md) for setup instructions.
Refer to [Organization scope](./organizations.md) for the permissions and the upgrade behavior.

### System prompt

Administrators can set a system prompt that applies to all agent sessions. This
is useful for establishing organizational conventions: coding standards,
commit message formats, preferred libraries, or repository-specific context.

This setting is available under **Admin settings** > **AI** > **Coder Agents** > **Instructions** and is only accessible to administrators. Developers do not see or interact with it.

### Plan mode instructions

Administrators can add deployment-wide instructions that apply only when a chat
enters plan mode. These instructions supplement the built-in planning behavior
and are useful for organization-specific planning requirements such as required
plan sections, approval checkpoints, or review workflows.

This setting is available under **Admin settings** > **AI** > **Coder Agents** > **Instructions**. Developers do not edit it directly.

The same value is exposed over the chat configuration API:

- `GET /api/v2/chats/config/plan-mode-instructions`
- `PUT /api/v2/chats/config/plan-mode-instructions`

### Template routing

Platform teams control which templates are available to agents and how the agent
selects them. When a developer describes a task, the agent reads template
descriptions to determine which template to provision.

By writing clear template descriptions, for example, "Use this template for
Python backend services in the payments repo", platform teams can guide the
agent toward the correct infrastructure without requiring developers to
understand template selection at all.

Administrators can also restrict which templates are available to agents at **Admin settings** > **AI** > **Coder Agents** > **Templates**.
Use the switch for each template in the list.
The same control is available on each individual template's settings page as **Allow Coder Agents to create workspaces using this template**.
Templates allow agents by default.
When you disable the control, the agent cannot read the template or provision workspaces from it.
This is separate from what developers observe when manually creating workspaces, so you can apply stricter policies to agent-created workspaces without affecting the manual workspace experience.

The same control is available outside the dashboard.
Use `coder templates create --agents-allowed=false` or `coder templates edit --agents-allowed=false <template>` for a single template.
Use the search filter `agents-allowed:false` on `GET /api/v2/templates` to list the templates that block agents.

See [Template Optimization](./template-optimization.md) for best practices on writing
discoverable descriptions, restricting template visibility, configuring network
boundaries, scoping credentials, and designing template parameters for agent
use.

### MCP servers

Organization admins can register external MCP (Model Context Protocol) servers that
provide additional tools for agent chat sessions. This includes configuring
authentication, controlling which tools are exposed via allow/deny lists, and
setting availability policies that determine whether a server is mandatory,
opt-out, or opt-in for each chat.

Each organization has its own set of MCP servers.

Refer to [MCP Servers](./mcp-servers.md) for configuration details.

### Workspace autostop fallback

Administrators can set a default autostop timer for agent-created workspaces
that do not define one in their template. Template-defined autostop rules always
take precedence. Active conversations extend the stop time automatically.

This setting is available under **Admin settings** > **AI** > **Coder Agents** > **Lifecycle**.
The maximum configurable value is 30 days.
When disabled, workspaces follow their template's autostop rules (or none, if the template does not define any).

### Concurrent agents

Community licenses support up to 5 concurrently active agents.
Coder doesn't limit how long those agents can run or how many tasks they complete over time.
Additional agents queue until an agent session becomes available.
With concurrent agents, individuals and small teams can experiment with Coder Agents at no cost.

Queued agents show a banner in the chat and start automatically when capacity frees.
Subtasks delegated by an agent don't count toward this limit.
Those subtasks run in a separate pool of up to 10 concurrent subtasks.

Premium deployments can purchase Agent Hours with their Premium license.
Agent Hours are shared across the deployment, and agents can run concurrently unless the Agent Hours hard limit is reached.
If the Agent Hours allocation is exhausted without a configured hard limit, Coder warns about usage but does not impose a concurrency limit.
When the Agent Hours hard limit is reached, additional agents queue under the concurrency limit.

### Spend management

AI Gateway budgets cap each user's AI spend, including Coder Agents chats, over a monthly period.
Coder sets budgets per group, and the deployment policy selects the group with the largest spend limit when a user belongs to several budgeted groups.
A per-user override takes priority over all group budgets.

Budgets are the only spend cap for Coder Agents chats.
Chats no longer enforce a separate limit of their own, and existing native limit values are not migrated to budgets.
Budget controls in the Coder UI, the group budget endpoints, and the AI spend status endpoints all require a license that includes AI Gateway.

Refer to [Spend management](./spend-management.md) for details.

### Git providers

Coder Agents leverages your existing
[external authentication](../../../admin/external-auth/index.md) configuration
to power the in-chat diff viewer. Self-hosted GitHub Enterprise deployments
require additional configuration for this feature.

See [Git Providers](./git-providers.md) for details.

### Data retention

Administrators can configure a retention period for archived conversations.
When enabled, archived conversations and orphaned files older than the
retention period are automatically purged. The default is 30 days.

This setting is available under **Admin settings** > **AI** > **Coder Agents** > **Lifecycle**.
Refer to [Data Retention](./chat-retention.md) for details.

### Experiments

Administrators enable experimental features using the `--experiments` flag on
`coder server` (or the `CODER_EXPERIMENTS` environment variable). Once enabled,
runtime configuration for those features is available under **Admin settings** >
**AI** > **Coder Agents**.

See the following pages for experiment-gated features:

- [Advisor](./advisor.md) (`--experiments=chat-advisor`)
- [Virtual desktop](./virtual-desktop.md) (`--experiments=chat-virtual-desktop`)

For chat debug logging (not experiment-gated), see [Chat debug logging](./chat-debug-logging.md).

## Where we are headed

The controls above cover providers, models, system prompts, templates, MCP servers, AI Gateway budgets, and data retention.
We are continuing to invest in platform controls based on what feedback we get from customers deploying agents in regulated and enterprise environments.

### Infrastructure-level enforcement

We believe that security-critical behaviors should not depend on the system
prompt. A system prompt can instruct an agent to "always format branch names like... ," but there is no guarantee the agent will comply every time.

For controls that matter — network boundaries, git push targets, allowed
hostnames — we intend to enforce them at the infrastructure and network layer.
Examples of what this looks like:

- **Network-restricted templates for agent workloads.** Because the AI comes
  from the control plane, agent workspaces do not need outbound access to LLM
  providers. You can create templates that only permit access to your git
  provider and nothing else.

## Why we take this approach

The common pattern in the industry today is that each developer installs and
configures their own coding agent inside their development environment. This
creates several problems for platform teams:

- **No standardization.** Different developers use different agents with
  different configurations. There is no unified way to enforce conventions or
  improve the experience across the organization.
- **Security is ad-hoc.** If the agent runs inside the workspace, it has access
  to whatever the workspace has access to — API keys, network endpoints,
  credentials. Restricting this requires per-workspace configuration that is
  difficult to maintain at scale.
- **Feedback is anecdotal.** Without centralized analytics, platform teams have
  no way to know which models perform best, which prompts cause failures, or how
  much agents are costing the organization.
- **Configuration is a developer burden.** Developers — especially those who
  are not power users — should not need to think about which agent to install,
  which API key to use, or how to configure a system prompt. They should
  describe the work they want done.

As models improve and the differences between agent harnesses continue to
shrink, we believe the leverage shifts toward user experience and platform-level controls: which
models to offer, how to enforce security, and how to use analytics to
continuously improve the development experience across the organization.
