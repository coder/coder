# Development Workflows and Guidelines

Use this guide for the development server, git workflow, hooks, contribution
metadata, new-feature checks, and API endpoint guardrails. Topic guides own Go,
testing, database, and OAuth2 procedures.

## New Feature Checklist

Before coding:

- [ ] Update the branch from its remote base.
- [ ] Identify database work. Follow [Database Development Patterns](DATABASE.md).
- [ ] Identify audit logging changes. Follow the database guide.
- [ ] Identify authorization and OAuth2 requirements. Follow
      [OAuth2 Development Guide](OAUTH2.md).
- [ ] Read nearby implementations and tests before choosing a pattern.

Before finishing:

- [ ] Add or update tests for changed behavior.
- [ ] Run targeted tests, then the relevant broader checks.
- [ ] Run `make fmt` and `make lint` when the changed area requires them.
- [ ] Confirm generated files, migrations, and API documentation are current.
- [ ] Review the final diff for unrelated edits.

## Development Server

Start local development with:

```sh
./scripts/develop.sh
```

Do not replace it with a manual build and `coder server --dev`. The script sets
up the development build, URL, local state, and rebuild loop. Use the URL printed
by the script.

For concurrent worktrees, port overrides, database reset options, and cleanup,
see [Development Isolation Guide for Agents](DEV_ISOLATION.md).

## Git Workflow

For an existing remote branch:

```sh
git fetch origin
git checkout branch-name
git pull origin branch-name
```

Make focused changes, inspect `git diff`, commit, and push normally. Do not force
push unless the user explicitly requests a history rewrite.

### Git Hooks

Install the repository hooks once per worktree:

```sh
git config core.hooksPath scripts/githooks
```

Never bypass hooks with `--no-verify`, and never change `core.hooksPath` to
disable them. Fix hook failures and retry.

The first run can be slow while Go, Node, and generation caches warm. A commit or
push may appear idle while a hook runs. Wait for it to finish, do not interrupt
or retry it merely because it is slow.

- `pre-commit` classifies staged files and runs `make pre-commit` or
  `make pre-commit-light`. Set `CODER_HOOK_RUN_ALL=1` to request the full target.
- `pre-push` runs the allowlisted `make pre-push` checks for relevant changes.
  A full run can take at least 15 minutes.

## Commit and PR Titles

Use `type(scope): message` for commit and PR titles. CI validates PR titles.

- Use one of `feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`, `build`,
  `ci`, `chore`, or `revert`.
- A scope must be a real filesystem path, or file stem, that contains every
  changed file.
- Use a broader path scope or omit the scope when no single path contains all
  changed files.
- Write a concise imperative message, normally about 70 characters or fewer.
- See [CONTRIBUTING.md](../../docs/about/contributing/CONTRIBUTING.md#commit-messages)
  for the full rules.

## API Design Guardrails

For every new public HTTP endpoint:

1. Define public request and response types in `codersdk/`.
2. Add the handler and register its route.
3. Add tests for success, authorization, validation, and failure behavior.
4. Add swagger annotations in the same change as the handler.

Prefer path parameters for user-scoped or resource-scoped routes when that
matches nearby routes. For experimental or unstable API paths, place
`// @x-apidocgen {"skip": true}` after the `@Router` annotation so the route is
excluded from the published API reference until it stabilizes.

Follow [OAuth2 Development Guide](OAUTH2.md) for protocol errors and public
endpoints that require restricted system access.
