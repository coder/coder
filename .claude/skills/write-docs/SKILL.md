---
name: write-docs
description: Authoring workflow for writing, moving, or restructuring Coder documentation under docs/. Applies the canonical content and prose guides, then verifies implementation details, Diátaxis mode, structure, and validation. Counterpart to doc-check, which reviews changes for documentation needs.
---

# Documentation Authoring Skill

Author or edit user-facing documentation under `docs/`. The [`doc-check` skill](../doc-check/SKILL.md) decides whether changes need docs; this skill authors them.

## Canonical sources

Read these before writing:

- [Content guidelines](../../../docs/.style/content-guidelines.md) govern scope, routing, Diátaxis, screenshots, structural rules, and what belongs in `docs/`. They supersede all other guidance on conflicts.
- [Prose style guide](../../../docs/.style/style-guide/README.md) governs wording and formatting. Read every section before editing `docs/`.
- [Pull request skill](../../../.agents/skills/pull-requests/SKILL.md) governs PR titles, descriptions, lifecycle, and CI follow-up.

Do not copy canonical rules into this skill. Link to the owner so rules cannot drift.

## Workflow

1. **Establish ground truth.**
   - Read the implementation, tests, issue, linked PRs, and nearby docs.
   - Verify routes in `coderd/coderd.go`, RBAC actions in `coderd/rbac/`, CLI or serpent definitions, frontend behavior, API parameters, defaults, thresholds, and error text as relevant.
   - Run documented commands in the reader's environment. Record real output. Flag anything you cannot execute or source-verify.
   - Confirm whether integrations already work without setup. Do not invent steps.
2. **Route the content.** Apply the content guide's [quick decision checklist](../../../docs/.style/content-guidelines.md#quick-decision-checklist) and [routing table](../../../docs/.style/content-guidelines.md#routing-table). Ask instead of guessing when ownership is unclear.
3. **Select one Diátaxis mode.** Choose tutorial, how-to, reference, or explanation. Keep one audience and one outcome per page.
4. **Plan structure.** Find the correct `docs/manifest.json` slot before drafting. Follow the canonical [structural rules](../../../docs/.style/content-guidelines.md#structural-rules) for new pages, generated content, Premium pages, moves, and redirects.
5. **Draft the smallest complete path.**
   - Give each code block an explicit action and expected result.
   - Inline only the code the reader applies. Put optional full-file references after the task.
   - Keep the happy path on one page. Link out for depth, not required steps.
   - Mirror parallel UI and CLI paths when both are supported.
   - For tutorials, let the reader act and observe before explaining the mechanism.
6. **Use screenshots only when the topic is confusing without one.** Do not organize sections around screenshots or add screenshot placeholders. Follow the content guide's [screenshot rules](../../../docs/.style/content-guidelines.md#what-belongs-in-the-docs).
7. **Review maintenance cost.** Prefer exact identifiers and durable references over hard-coded line numbers, screenshots of copyable text, or duplicated vendor guidance.
8. **Validate and hand off.** Apply the prose guide with it open, run the checks below, inspect the final diff, and keep the PR focused.

## Validation

Run:

```sh
pnpm run format-docs
pnpm run lint-docs
make lint/prose
make lint/emdash
```

Also:

- Execute commands and code examples, or identify them as unverified.
- Confirm links and moved anchors resolve.
- For a new or moved page, confirm its manifest path exists exactly once and `jq empty docs/manifest.json` passes.
- For a rename, update inbound links and coordinate the redirect in `coder/coder.com` so the old public path does not break between merges.
- Do not edit generated CLI reference pages directly.

## Handoff checklist

- [ ] Claims match code, tests, or observed output.
- [ ] Content belongs in `docs/` and uses one Diátaxis mode.
- [ ] Commands include expected results and were tested or flagged.
- [ ] Prose, formatting, links, manifest, Premium state, and redirects are correct.
- [ ] Screenshot use follows the canonical minimal-use policy.
- [ ] The PR title and body follow the pull request skill.
