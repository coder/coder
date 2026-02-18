# AI Governance Deployment Guide

This guide provides recommended deployment patterns for Coder's AI Governance
features — [AI Bridge](./ai-bridge/index.md) and
[Agent Boundaries](./agent-boundaries/index.md) — based on common enterprise
use cases.

## Deployment patterns

### Regulated enterprise (financial services, healthcare)

**Goal:** Obtaining complete audit trails, data exfiltration prevention, and provable
controls towards meeting compliance requirements.

**Recommended deployment:**

- **AI Bridge**: Enable with provider keys. If you have an existing LLM gateway,
  point Bridge at it as the upstream. Enable structured logging and OTEL export
  to your SIEM.
- **Agent Boundaries**: Enable via template with a restrictive allowlist.
  Restrict to internal GitHub/GitLab, approved package registries, and internal
  services. 
- **Template configuration**: Pre-inject Bridge base URLs and session tokens.
  Mount Boundary `config.yaml`. Developers should see zero additional setup.

**What this gives you:**

- Every AI interaction is logged and attributed — user, model, prompt, tools,
  timestamp.
- Agents cannot reach unauthorized endpoints.
- Audit logs stream to your SIEM for incident investigation and compliance
  reporting.
- Defense-in-depth: Bridge handles identity and audit; Boundaries handle network
  isolation.

### AI tool standardization

**Goal:** Consolidate AI tool access onto a governed gateway. Eliminate shadow
AI and unmanaged API key sharing.

**Recommended deployment:**

- **AI Bridge** (primary focus): Configure server-side with enterprise API keys.
  Set up templates to override all provider base URLs so agents route through
  Bridge automatically.
- **Agent Boundaries** (secondary): Use to block direct access to provider APIs
  (`api.openai.com`, `api.anthropic.com`, etc.) from workspaces, forcing all AI
  traffic through Bridge.

**What this gives you:**

- Single control point for all AI tool access.
- Elimination of per-developer API key management.
- Visibility into which tools, models, and providers developers actually use.
- Token-level cost attribution for chargeback and optimization.

### Autonomous agent workloads

**Goal:** Run headless agents via [Coder Tasks](./tasks.md) for automated code
review, documentation, or issue resolution — with strict guardrails and no
human-in-the-loop during execution.

**Recommended deployment:**

- **Agent Boundaries** (critical): Agent-only workloads should always have
  Boundary enabled. Define restrictive allowlists per template. Autonomous agents
  need tighter controls than human+agent workloads.
- **AI Bridge**: Enable for full session logging. Every autonomous agent
  interaction is audited.
- **Tasks**: Use Coder Tasks as the execution engine for lifecycle control and
  notification hooks.

**What this gives you:**

- Agents run autonomously within explicit guardrails.
- Template admins control what agents can access; developers don't configure
  Boundary.
- All policy decisions stream to the control plane for security review.
- Different templates can have different policies (e.g., research agents get
  broader access; deployment agents get narrow access).

## Phased rollout

For organizations evaluating AI Governance, we recommend a phased approach:

### Phase 1: AI Bridge (weeks 1–2)

1. Confirm HTTP proxy path to model providers is open (if applicable).
2. Enable AI Bridge on the Coder deployment.
3. Configure 1–2 provider keys (e.g., Anthropic + OpenAI).
4. Set up one template with Bridge base URLs injected.
5. Validate with a supported AI client (Claude Code, Codex, etc.).
6. Demonstrate audit logging in the Coder UI or server logs.

### Phase 2: Agent Boundaries (weeks 2–3)

1. Identify 5–10 internal tools and domains agents need to access.
2. Create an initial `config.yaml` with those domains.
3. Enable Boundary in the template module (`enable_boundary = true`).
4. Test with your specific agent tooling.
5. Walk through audit log output.
6. Demonstrate a blocked request scenario.

### Phase 3: Combined governance (weeks 3–4)

1. Deploy Bridge + Boundaries together in a production-like template.
2. Bridge forces all AI traffic through the governed gateway.
3. Boundaries restrict agent network access.
4. Export logs to your SIEM for correlation.
5. Present the combined governance posture to your security team.

## Pre-deployment checklist

Before enabling AI Governance:

- [ ] Confirm HTTP proxy path to model providers is open.
- [ ] Identify internal tools and domains agents will need to access.
- [ ] Create an initial Boundary allowlist.
- [ ] Test CLI wrapper mechanics in your environment.
- [ ] Confirm that per-template scoping matches your infrastructure model.
- [ ] Note that Boundaries operate at the process level — escalate to your
      network security team if network-wide filtering is also required.

## Next steps

- [AI Bridge Setup](./ai-bridge/setup.md) — Enable Bridge and configure
  providers.
- [Agent Boundaries](./agent-boundaries/index.md) — Configure process-level
  network policies.
- [AI Bridge Monitoring](./ai-bridge/monitoring.md) — Set up observability.
