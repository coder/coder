---
name: dogfood
description: "Dogfood a Coder PR: inspect the PR context, check out the right branch or stack, decide what to validate, run the instance with the verify-coder skill, validate the changed functionality with UI evidence when needed, and report findings."
---

# Coder dogfood

Use this skill when the user asks to dogfood, UAT, manually validate, or end-to-end test a Coder PR, branch, or stack.

This skill owns the scoping and the report: what the PR changed, which scenarios to validate, and what to post. The `verify-coder` skill (`.agents/skills/verify-coder/`) owns the run: launch, readiness, driving the UI and the CLI, evidence folders, and cleanup. Do not start `scripts/develop.sh` by hand in a checkout where you use `verify-coder`.

Do not hardcode a large scenario plan into this skill. Derive scenarios from the PR description, changed files, tests, docs, and the user's requested focus.

## References

Background on startup, isolation, observability, and troubleshooting:

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
6. Resolve the target PR, base PR, stack order, and required access from conversation context, Git state, PR metadata, and available local configuration. Ask only if essential target or access information remains unavailable, and continue validation that does not depend on it. Never expose credential values while investigating.

## Run the instance

Follow `verify-coder`: `scripts/launch.sh`, then `scripts/doctor.sh`, then the journeys or hand-driven checks for the scenarios you chose, then `scripts/cleanup.sh`. Its doctor is the readiness check, and its feature files under `features/` show how to drive each area.

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

`verify-coder` starts the server without the host's provider keys, so nothing spends tokens by itself. `features/coder-agents.md` there sets up a provider against a local fake model. When the PR needs a real model, add the provider through the UI with a key from the dogfood environment.

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
2. Run `verify-coder`'s `scripts/launch.sh` on that version.
3. Create representative state required by the PR.
4. Run `scripts/cleanup.sh --keep-data`, which stops the server and keeps the dev database.
5. Check out the target migration PR, branch, or SHA.
6. Run `scripts/launch.sh` again against the same preserved state.
7. Verify that the PR-specific state migrated and still works.

The exact migration checks should come from the PR context. Do not use a generic migration checklist as a substitute for reading the PR.

## Evidence to capture

`verify-coder` keeps screenshots, read-backs and logs in its evidence folders. The report also needs:

- PR number and URL.
- Branch and SHA tested.
- Provider and model names, without secrets.
- Validation scenarios run, with prompts and outcomes for chat tests.
- Tool calls attempted and results.
- SQL queries and results, when database state matters.
- What was not tested.

## PR comments

Only post to GitHub when the user asked for it or explicitly allowed it.

Keep comments concise:

```markdown
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
- Which `verify-coder` launch and checks ran.
- Which providers and models were configured.
- Which scenarios passed.
- Which scenarios failed.
- What was not tested.
- Where evidence is stored.
- Whether `scripts/cleanup.sh` passed.
