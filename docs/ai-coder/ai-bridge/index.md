# AI Bridge

![AI bridge diagram](../../images/aibridge/aibridge_diagram.png)

AI Bridge is a smart gateway for AI that runs in-memory within the Coder control
plane (`coderd`). It intercepts all LLM API traffic between your users' coding
agents/IDEs and upstream model providers (OpenAI, Anthropic, AWS Bedrock, and
custom endpoints), providing centralized authentication, audit logging,
token/cost tracking, and MCP tool injection — without changing developer
workflows.

AI Bridge solves four key problems:

1. **Centralized authentication**: Replace per-developer API key distribution
   with a single enterprise key per provider. Developers authenticate via their
   existing Coder session tokens — they never hold provider API keys.
1. **Auditing and attribution**: Every prompt, token usage, tool invocation, and
   model selection is logged and attributed to a specific user, whether the
   interaction is human-initiated or autonomous.
1. **Cost attribution**: Track token usage per user, per model, per provider for
   chargeback, optimization, and spend visibility.
1. **Centralized MCP administration**: Define approved MCP servers and tools
   centrally. Agents receive tools automatically — no per-developer MCP
   configuration required.

## Architecture

AI Bridge runs as an in-process component within `coderd` (the Coder control
plane). It does not require a separate deployment or infrastructure.

### Request lifecycle

1. A template admin configures AI Bridge base URLs and session tokens in the
   workspace template (Terraform). This propagates to every workspace
   automatically.
2. The AI client (Claude Code, Codex, Cline, etc.) sends API requests to the
   Bridge endpoint using the developer's Coder session token — not a provider
   API key.
3. Bridge authenticates the user via Coder's identity system.
4. Bridge logs the prompt and request metadata.
5. If MCP servers are configured, Bridge fetches available tools and injects them
   into the request.
6. Bridge forwards the modified request to the upstream LLM provider using the
   enterprise API key.
7. The LLM response (including any tool calls) flows back through Bridge.
8. If the response includes MCP tool calls, Bridge executes them on behalf of
   the user and sends results back to the LLM.
9. Bridge logs usage (tokens, tools, model, user) and returns the final response
   to the agent.

### Key architectural points

- **Developers never hold provider API keys.** They authenticate with their
  existing Coder credentials.
- **The enterprise holds one key per provider.** These are configured centrally
  in `coderd` via environment variables or Helm values.
- **Template-level configuration.** Configure AI Bridge once in a workspace
  template, and it propagates to every workspace created from that template.
- **Universal client compatibility.** Works with any AI tool that supports base
  URL overrides, including Claude Code, Codex, Cline, Zed, and more. For tools
  that don't support base URL overrides, [AI Bridge Proxy](./ai-bridge-proxy/index.md)
  can intercept traffic transparently.

## When to use AI Bridge

As LLM adoption grows, administrators need centralized auditing, monitoring, and token management. AI Bridge is designed for organizations that need centralized management and observability over AI tool usage for thousands of engineers - all from a single control plane. 

Common scenarios include:

- **Measuring AI adoption** across teams, projects, or the entire organization
- **Establishing an audit trail** of prompts, tool invocations, and model usage
  for compliance and incident investigation
- **Managing token spend** with per-user, per-model cost attribution
- **Eliminating shadow AI** by routing all AI traffic through a governed gateway
  and blocking direct access to provider APIs
- **Standardizing AI tooling** across engineering teams with consistent
  configuration and approved models

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
- Boundaries prevent agents from reaching unauthorized endpoints (e.g., blocking
  direct access to `api.openai.com` or `api.anthropic.com` to force traffic
  through Bridge).
- Both produce structured logs that can be exported to your SIEM for correlated
  analysis.

> **Note:** Correlating Bridge and Boundary log streams currently requires
> exporting both to an external analytics platform and joining on shared fields
> (user, workspace, timestamp). Built-in cross-referencing is planned for a
> future release.

## Current scope and limitations

AI Bridge is under active development. The following describes the current scope
to help you plan your deployment:

- **Supported clients**: AI Bridge works with any client that supports base URL
  overrides. See the [client compatibility table](./clients/index.md#compatibility)
  for the full list. Some tools (Cursor, Windsurf, Sourcegraph Amp) do not
  currently support the required base URL override and authentication model.
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
