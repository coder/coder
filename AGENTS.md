# Coder Development Guidelines

Make the smallest correct change, follow existing patterns, and verify the result. Carry the requested task through implementation, verification, and necessary follow-up until it is complete or blocked by information or access you cannot obtain. Do not stop at a plan, partial fix, or offer to continue when the user requested completed work.

Prioritize correctness over agreement. State uncertainty instead of guessing, and push back on technically unsound requests with evidence.

## Autonomy and clarification

- Resolve routine ambiguity by inspecting relevant code, tests, documentation, and history. Make reasonable, reversible assumptions consistent with the user's intent and existing patterns; state consequential assumptions and continue working.
- Ask only when essential information cannot be recovered from available context and would materially change the result, or when a destructive or irreversible action requires authorization the user has not already provided. Reuse authorization from the conversation instead of asking again for the same action.
- If clarification or approval is required, continue authorized work that does not depend on the answer. Explain the specific blocker and what you have already investigated.
- Apply repository guidance within its stated scope and honor explicit user instructions. Do not turn optional recommendations or routine implementation choices into approval requirements.

## Task-specific guidance

Load only the guidance relevant to the task:

| Scope                                               | Guidance                                                |
|-----------------------------------------------------|---------------------------------------------------------|
| Development servers, Git, hooks, and routine checks | [WORKFLOWS.md](.claude/docs/WORKFLOWS.md)               |
| API endpoints and Swagger                           | [WORKFLOWS.md](.claude/docs/WORKFLOWS.md)               |
| Go                                                  | [GO.md](.claude/docs/GO.md)                             |
| Tests and concurrency                               | [TESTING.md](.claude/docs/TESTING.md)                   |
| Database and SQLC                                   | [DATABASE.md](.claude/docs/DATABASE.md)                 |
| OAuth2 and authorization                            | [OAUTH2.md](.claude/docs/OAUTH2.md)                     |
| Architecture                                        | [ARCHITECTURE.md](.claude/docs/ARCHITECTURE.md)         |
| Troubleshooting                                     | [TROUBLESHOOTING.md](.claude/docs/TROUBLESHOOTING.md)   |
| Observability                                       | [OBSERVABILITY.md](.claude/docs/OBSERVABILITY.md)       |
| Isolation, ports, and cleanup                       | [DEV_ISOLATION.md](.claude/docs/DEV_ISOLATION.md)       |
| Failure reports                                     | [AGENT_FAILURES.md](.claude/docs/AGENT_FAILURES.md)     |
| PR descriptions                                     | [PR_STYLE_GUIDE.md](.claude/docs/PR_STYLE_GUIDE.md)     |
| Existing docs prose                                 | [docs style guide](docs/.style/style-guide/README.md)   |
| Docs scope and routing                              | [content guidelines](docs/.style/content-guidelines.md) |
| Docs structure and research                         | [DOCS_STYLE_GUIDE.md](.claude/docs/DOCS_STYLE_GUIDE.md) |
| New, moved, or restructured docs                    | [write-docs skill](.claude/skills/write-docs/SKILL.md)  |
| Frontend                                            | [site/AGENTS.md](site/AGENTS.md)                        |

For changes under `site/src/`, also read [FRONTEND_PATTERNS.md](.claude/docs/FRONTEND_PATTERNS.md). For chatd work, read [coderd/x/chatd/ARCHITECTURE.md](coderd/x/chatd/ARCHITECTURE.md). When the docs style guide and the content guidelines conflict, the content guidelines govern scope and routing.

## Workflow

- Inspect the working tree before editing. For an existing PR, check out its branch first.
- Follow existing architecture for routine decisions. For requested architectural work, investigate options, choose a reasonable approach, and explain the tradeoffs while proceeding. Ask before introducing major architectural changes outside the requested scope.
- Answer informational questions directly. Requests to implement, fix, or investigate authorize that work even when phrased as a question.
- Install and use the repository Git hooks. Never bypass them with `--no-verify`. Wait for slow first runs while caches warm.
- Prefer targeted tests and checks while iterating. Run the broader checks required by the affected area before handoff.
- Do not force-push unless explicitly requested.
- Commit and PR titles use `type(scope): message`. A scope must be a real path containing every changed file. Use a broader scope or no scope for cross-cutting changes.

## Essential commands

| Task              | Command                  |
|-------------------|--------------------------|
| Develop           | `./scripts/develop.sh`   |
| Build             | `make build`             |
| Build slim        | `make build-slim`        |
| Test              | `make test`              |
| Test one          | `make test RUN=TestName` |
| Race test         | `make test-race`         |
| Lint              | `make lint`              |
| Generate          | `make gen`               |
| Format            | `make fmt`               |
| Pre-commit checks | `make pre-commit`        |
| Pre-push checks   | `make pre-push`          |

Docs use `pnpm run format-docs` and `pnpm run lint-docs`. Frontend commands live in `site/AGENTS.md`.

## Repository guardrails

- **Database changes:** edit `coderd/database/queries/*.sql`, run `make gen`, update `enterprise/audit/table.go` for audit errors, then run `make gen` again.
- **New resources:** scope every new resource to an organization (`organization_id` column, organization-scoped RBAC and routes), never deployment-wide.
- **OAuth2:** return RFC-compliant errors such as `writeOAuth2Error(...)`. Public endpoints that need system access use `dbauthz.AsSystemRestricted`.
- **Chatd:** when a change affects the documented architecture, do not edit the architecture document yourself. Leave TODO items in the affected sections; the human PR author writes the actual updates.
- **Public API:** add the required Swagger annotations for new public HTTP endpoints.
- **Transactions:** keep `InTx` work on the transaction handle. Prefer explicit database-to-SDK converters.
- **Concurrent tests:** call `t.Parallel()`, use unique identifiers, and do not use `time.Sleep` to mask timing problems.
- **Frontend:** reuse shared UI primitives. Cover visual states with Storybook stories; Pixel screenshots them in CI. Cover component behavior with Vitest, React Testing Library and `userEvent` tests that assert the non-visual outcome of the interaction (callback, request, state); extend existing coverage instead of adding a new test when equivalent coverage already exists.
- **GitHub Actions:** set top-level `permissions: {}` and grant only required permissions per job.

## Code and writing style

- Follow the [Uber Go Style Guide](https://github.com/uber-go/guide/blob/master/style.md) for Go code.
- Use language-server navigation when available.
- Name code for what it does, not its implementation or history. Wrap errors with context.
- Document exported symbols with idiomatic Go doc comments or JSDoc.
- Avoid unrelated edits. Preserve comments that explain non-obvious behavior.
- Comments must be concise and substantive. Explain behavior, constraints, or rationale, not the history of the edit.
- Do not use em dashes, en dashes, or spaced double hyphens as punctuation in code, comments, strings, or documentation.
- Ensure files end with a newline.

## Local configuration

Read `AGENTS.local.md` when present. It may be gitignored and is not imported automatically.
