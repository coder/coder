# AI Bridge

![AI bridge diagram](../../images/aibridge/aibridge_diagram.png)

AI Bridge is a smart AI gateway. It sits as an intermediary between your users' coding agents / IDEs and providers like OpenAI and Anthropic. It intercepts all LLM API traffic between your users' coding agents/IDEs and upstream model providers (OpenAI, Anthropic, AWS Bedrock, and other compatible providers), providing centralized authentication, audit logging, token/cost tracking, and MCP tool injection — without changing the developer workflows.

AI Bridge solves four key problems:

1. **Centralized authentication**: Replace per-developer API key distribution with a single enterprise key per provider. Developers authenticate via their existing Coder session tokens — they never hold provider API keys.
1. **Auditing**: Every prompt, tool invocation, and model selection is logged and attributed to a specific user, whether the interaction is human-initiated or autonomous.
1. **Cost attribution**: Track token usage per user, per model, per provider for chargeback, optimization, and spend visibility.
1. **Centralized MCP administration**: Define approved MCP servers and tools centrally. Agents receive tools automatically — no per-developer MCP configuration required.

## When to use AI Bridge

As LLM adoption grows, administrators need centralized auditing, monitoring, and token management. AI Bridge is designed for organizations that need centralized management and observability over AI tool usage for thousands of engineers - all from a single control plane. 

Common scenarios include:

- **Measuring AI adoption** across teams, projects, or the entire organization
- **Establishing an audit trail** of prompts, tool invocations, and model usage for compliance and incident investigation
- **Managing token spend** with per-user, per-model cost attribution
- **Eliminating shadow AI** by routing all AI traffic through a governed gateway
- **Standardizing AI tooling** across engineering teams with consistent configuration and approved models

### Multi-provider and existing gateway integration

Organizations with existing LLM gateway infrastructure (such as LiteLLM,
Portkey, or an internal AI gateway) can point AI Bridge at their gateway as the
upstream provider. In this pattern, Bridge handles identity attribution and audit
logging while the existing gateway handles routing, load balancing, and failover.

Bridge is complementary to existing gateways, not competitive with them.

## Using AI Bridge with Agent Boundaries

AI Bridge and [Agent Boundaries](../agent-boundaries/index.md) are independent
features that deliver maximum value when deployed together:

- **AI Bridge** governs the *LLM layer* — identity, audit, cost, and tool
  access.
- **Agent Boundaries** governs the *network layer* — restricting which domains
  and endpoints agents can reach.

Together, they provide defense-in-depth:

- Bridge ensures all AI traffic is authenticated, logged, and attributed.
- Boundaries prevent agents from reaching unauthorized endpoints. 
- Both produce structured logs that can be exported to your SIEM for correlated
  analysis.

> **Note:** Correlating Bridge and Boundary log streams currently requires
> exporting both to an external analytics platform and joining on shared fields
> (user, workspace, timestamp). 

## Current scope and limitations

AI Bridge is under active development. The following describes the current scope
to help you plan your deployment:

- **Supported clients**: AI Bridge works with any client that supports base URL
  overrides. See the [client compatibility table](./clients/index.md#compatibility)
  for the full list. Some tools (Cursor, Windsurf, Sourcegraph Amp) do not
  currently support the required base URL override and authentication model. In these cases, we recommend usage of the [AI Bridge Proxy](./ai-bridge-proxy/index.md).
- **RBAC**: All users with the `member` role can currently use AI Bridge.
  Fine-grained role-based access control (e.g., `AI Bridge User` permissions)
  is planned.
- **Custom metadata**: Support for custom metadata headers to tag interactions
  with project, feature, or compliance labels is in development.
- **Independent deployment**: AI Bridge currently runs in-process within
  `coderd`. The ability to run Bridge as a standalone service is planned for a
  future release.

## Next steps

- [Setup](./setup.md) — Enable AI Bridge and configure provider API keys.
- [Client Configuration](./clients/index.md) — Configure AI tools to route
  through Bridge.
- [MCP Tools Injection](./mcp.md) — Centrally manage MCP servers and tool
  access.
- [AI Bridge Proxy](./ai-bridge-proxy/index.md) — Support tools that don't allow base
  URL overrides.
- [Monitoring](./monitoring.md) — Export logs, metrics, and traces for
  observability.
- [Reference](./reference.md) — Technical reference for AI Bridge configuration
  options.

<children></children>

  for the full list. Some tools (e.g., Cursor, Windsurf, Sourcegraph Amp etc.) do not
> [!NOTE]
> Correlating Bridge and Boundary log streams currently requires
> exporting both to an external analytics platform and joining on shared fields
> (user, workspace, timestamp). Built-in cross-referencing is planned for a
> future release.