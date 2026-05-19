---
name: dogfood
description: "Run a Coder PR dogfood instance: inspect PR context, check out the right branch or stack, start Coder with scripts/develop.sh using agent-safe dev-instance practices, validate the changed functionality with UI evidence when needed, and report findings."
---

# Coder dogfood

Use this skill when the user asks to dogfood, UAT, manually validate, or end-to-end test a Coder PR, branch, or stack.

The primary job is to run a reliable local dogfood instance and use the PR context to decide what to validate. Do not hardcode a large scenario plan into this skill. Derive scenarios from the PR description, changed files, tests, docs, and the user's requested focus.

## References

Use the canonical repo guidance for startup, isolation, observability, and cleanup:

- `.claude/docs/WORKFLOWS.md`
- `.claude/docs/DEV_ISOLATION.md`
- `.claude/docs/OBSERVABILITY.md`
- `.claude/docs/TROUBLESHOOTING.md`

## Understand the target first

Before starting Coder:

1. Identify the PR, branch, stack, or SHA to test.
2. Read the PR title and description.
3. Inspect the changed files and relevant tests.
4. Summarize what behavior changed.
5. Decide what must be validated through UI, API, SQL, logs, browser automation, desktop automation, or computer use.
6. Ask for clarification if the target PR, base PR, stack order, or required credentials are ambiguous.

## Start the dogfood instance

Use the development script:

```bash
./scripts/develop.sh
```

For isolated multi-worktree dogfood runs, prefer one of these:

```bash
CODER_DEV_PORT_OFFSET=true ./scripts/develop.sh
```

```bash
./scripts/develop.sh --port-offset
```

Pass extra Coder server flags after the delimiter argument named `--`. For trace logging, use `--trace` as the forwarded server flag.

Useful defaults:

| Resource | Default |
| --- | --- |
| API server | `3000` |
| Web UI | `8080` |
| Workspace proxy | `3010` |
| Coder metrics | `2114` |

Useful overrides:

- `CODER_DEV_PORT`
- `CODER_DEV_WEB_PORT`
- `CODER_DEV_PROXY_PORT`
- `CODER_DEV_PROMETHEUS_PORT`
- `CODER_DEV_PORT_OFFSET`
- `CODER_DEV_ACCESS_URL`
- `CODER_DEV_ADMIN_PASSWORD`

## Readiness

Do not start browser, desktop, or computer use validation until the instance is ready.

Accept either:

- `GET /healthz` succeeds.
- The develop script prints `Coder is now running in development mode`.

The banner is the preferred ready signal for UI work because it includes the effective API and Web UI URLs.

If readiness fails, inspect the develop output first, especially logs tagged:

- `api`
- `site`
- `proxy`
- `ext-provisioner`
- `prometheus`

Look for port conflicts, database recovery prompts, frontend build errors, and missing dependencies.

## Validate from the PR context

Do not run only generic flows. Validate the behavior changed by the PR.

Use the PR description and diff to choose scenarios such as:

- UI rendering and interaction.
- API behavior.
- SQL state and persistence.
- Server log behavior.
- Browser or desktop flows.
- Workspace or agent flows.
- Restart or resume behavior.
- Migration behavior when the user asks for migration validation.

Prefer repeatable API or SQL assertions for core correctness. Use computer use, desktop automation, or browser automation when the user asked for screenshots, the PR changes UI, or the workflow must be verified visually.

## When the PR touches Coder agents

If the PR affects Coder agents, AI chat, AI Gateway, AI Bridge, provider configuration, model configuration, tool calling, MCP, conversation persistence, or agent UI flows, validate the actual user workflow and not only backend APIs.

### Use computer use for UI validation

Use computer use, desktop automation, or browser automation to interact with the real Coder UI when validating agent behavior.

Validate through the UI when possible:

- Provider setup screens.
- Model setup screens.
- Model selection.
- Chat creation.
- Existing chat resume.
- Tool-call approval or execution flows.
- Error states shown to users.
- Loading, streaming, and completion states.
- Any new or changed UI copy.

Capture screenshots for important states, especially:

- Provider configuration, without secrets visible.
- Model list or selected model.
- Chat prompt and response.
- Tool-call execution or result.
- Error messages.
- Migrated or resumed conversations.

Do not rely only on direct API calls when the PR changes the user-facing agent experience. API checks are still useful for repeatability, but the dogfood result should include actual UI validation when the PR affects Coder agents.

### Provider and model setup

If provider or model setup is required, reuse the existing environment variables available in the dogfood environment to set up test providers and models.

Common provider types:

- Anthropic.
- OpenAI.
- OpenAI-compatible provider pointed at AI Bridge or AI Gateway, when relevant.

Rules:

- Reuse available environment variables for provider credentials.
- Never print, screenshot, commit, or post secret values.
- Do not include raw API keys in logs, PR comments, screenshots, shell history, or summaries.
- Prefer the smallest reliable models for routine dogfood testing.
- Prefer models with no thinking or extended reasoning enabled for routine validation.
- Use larger models, thinking models, or a specific model only when the PR behavior depends on that model configuration.
- If the user requested a specific model, configure and validate that model.
- Verify that each configured provider and model appears in the UI and can complete at least one basic conversation before deeper testing.

Example routine validation:

1. Configure Anthropic from available environment variables.
2. Configure OpenAI from available environment variables.
3. Add one small non-thinking Anthropic model.
4. Add one small non-thinking OpenAI model.
5. Start a new chat with each model.
6. Run a short multi-turn conversation.
7. If tool calling is in scope, run a simple tool-call scenario and verify the UI shows the correct state.

If the PR touches specific model configuration behavior, expand validation to cover that behavior. Examples include thinking budget, context window, model display name, provider-specific model IDs, tool-use support flags, OpenAI-compatible routing, AI Bridge or AI Gateway behavior, and migration from old provider or model structures.

## Migration or stack validation

Keep migration handling lightweight in this skill.

If the user asks for migration or stack UAT:

1. Record the pre-migration PR, branch, or SHA.
2. Start the dogfood instance on that version.
3. Create representative state required by the PR.
4. Stop the server without deleting the dev database or state.
5. Check out the target migration PR, branch, or SHA.
6. Start the dogfood instance against the same preserved state.
7. Verify that the PR-specific state migrated and still works.

The exact migration checks should come from the PR context. Do not use a generic migration checklist as a substitute for reading the PR.

## Evidence to capture

Capture enough evidence for another engineer to understand the result:

- PR number and URL.
- Branch and SHA tested.
- Start command and relevant flags.
- Effective API and Web UI URLs.
- Provider and model names, without secrets.
- Validation scenarios run.
- Prompts and outcomes for chat tests.
- Tool calls attempted and results.
- Screenshots, when UI validation was requested or useful.
- SQL queries and results, when database state matters.
- Relevant logs and errors.
- What was not tested.

## Cleanup

Use the least destructive cleanup that solves the problem.

Preferred order:

1. Stop the develop process gracefully with `Ctrl+C`.
2. If a port is stuck, identify the listener with `lsof -iTCP:<port> -sTCP:LISTEN` and terminate only that process.
3. For database issues, prefer develop flags such as `--db-rollback`, `--db-continue`, or `--db-reset`.
4. Only delete `.coderv2` state when that is truly intended.
5. If embedded Prometheus was used and remains stuck, stop the develop process first, then remove the `coder-prometheus` container if needed.

## PR comments

Only post to GitHub when the user asked for it or explicitly allowed it.

When posting on Mike's behalf, include:

```markdown
> Mux working on Mike's behalf
```

Keep comments concise:

```markdown
> Mux working on Mike's behalf

Dogfood results:

Passed:
- ...

Failed:
- ...

Not tested:
- ...

Evidence:
- PR/SHA: ...
- Start command: ...
- Providers/models: ...
- Screenshots/logs: ...

Reproduction:
1. ...
2. ...
3. ...
```

If testing a stack, comment on the PR where the issue appears to originate. If that is uncertain, say so.

## Final response checklist

Before responding to the user, report:

- What PR, branch, and SHA were tested.
- How the dogfood instance was started.
- Which URL was used.
- Which providers and models were configured.
- Which scenarios passed.
- Which scenarios failed.
- What was not tested.
- Where evidence is stored.
- Whether the server was stopped.
