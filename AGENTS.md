# Coder Development Guidelines

Make the smallest correct change, follow existing patterns, and verify the result. Ask only when the request is unclear, a meaningful design choice remains, or the action is destructive. If you want an exception to any rule in these documents, stop and get explicit permission first.

Prioritize correctness over agreement. State uncertainty instead of guessing, and push back on technically unsound requests with evidence.

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
- Discuss architectural decisions such as framework changes, major refactoring, and system design before implementing them. Routine fixes and clear implementations do not need discussion.
- When asked a question, answer the question instead of jumping to implementation.
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
- **Frontend:** reuse shared UI primitives and test components or pages through Storybook stories. Plain Vitest files are for pure logic only.
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
