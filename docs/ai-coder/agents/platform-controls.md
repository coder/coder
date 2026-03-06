# Platform Controls

## Design philosophy

Coder Agents is built on a simple premise: platform teams should have full
control over how agents operate, and developers should have zero configuration
burden.

This means:

- **All agent configuration is admin-level.** Providers, models, system prompts,
  and tool permissions are set by platform teams from the control plane. These
  are not user preferences — they are deployment-wide policies.
- **Developers never need to configure anything.** A developer just describes
  the work they want done. They do not need to pick a provider, enter an API
  key, or write a system prompt — the platform team has already set all of
  that up. The goal is not to restrict developers, but to make configuration
  unnecessary for a great experience.
- **Enforcement, not defaults.** Settings configured by administrators are
  enforced server-side. Developers cannot override them. This is a deliberate
  distinction — a setting that a user can change is a preference, not a policy.

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

Developers select from the set of models an administrator has enabled. They
cannot add their own providers, supply their own API keys, or access models that
have not been explicitly configured.

See [Models](./models.md) for setup instructions.

### System prompt

Administrators can set a system prompt that applies to all agent sessions. This
is useful for establishing organizational conventions — coding standards,
commit message formats, preferred libraries, or repository-specific context.

The system prompt configuration is only accessible to administrators in the
dashboard. Developers do not see or interact with it.

### Template routing

Platform teams control which templates are available to agents and how the agent
selects them. When a developer describes a task, the agent reads template
descriptions to determine which template to provision.

By writing clear template descriptions — for example, "Use this template for
Python backend services in the payments repo" — platform teams can guide the
agent toward the correct infrastructure without requiring developers to
understand template selection at all.

## Where we are headed

Coder Agents is in its early stages. The controls above — providers, models,
and system prompt — are what is available today. We are actively building
toward a broader set of platform controls based on what we are hearing from
customers deploying agents in regulated and enterprise environments.

The areas we are investing in include:

### Usage controls and analytics

We plan to give platform teams visibility into how agents are being used across
the organization: token consumption per user, cost per PR, merge rates by model,
and average time from prompt to merged pull request.

The goal is to let platform teams make data-driven decisions — like switching
the default model when analytics show one model produces higher merge rates —
rather than relying on anecdotal feedback from individual developers.

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
- **Template scoping for agents.** We intend to let administrators restrict
  which templates are available in the agentic interface, separate from what
  developers see when manually creating workspaces. This lets you apply stricter
  policies to agent-created workspaces without affecting the developer
  experience for manually-created ones.

### Tool customization

The agent ships with a standard set of tools (file read/write, shell execution,
sub-agents). We intend to let platform teams customize the available tool set —
adding organization-specific tools or restricting default ones — without
modifying agent source code.

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
