# Codebase fundamentals course

This one-day course teaches the fundamentals of the Coder codebase for new
contributors. It focuses on the code paths you need to understand before making
your first small full-stack change.

Use this course with a local checkout of `coder/coder`. The labs are designed to
work as tracing exercises or disposable training patches. Do not submit the
training patches as product changes unless you first turn them into a real issue
or PR.

## Learning outcomes

After this course, you should be able to:

- Explain how the CLI, control plane, database, provisioner, agent, Tailnet, and
  frontend fit together.
- Trace a browser action from a React page to a React Query hook, API function,
  SDK type, backend route, handler, authorization check, SQL query, and test.
- Identify when a change needs `make gen`, database migrations, dbauthz updates,
  audit table updates, Storybook coverage, or focused Go tests.
- Make a small disposable full-stack patch using existing code patterns.
- Prepare a first PR with the right checks, title format, and test plan.

## Before you start

Read or skim these docs first:

- [Contributing](./CONTRIBUTING.md)
- [Backend](./backend.md)
- [Frontend](./frontend.md)
- [Documentation](./documentation.md)
- [Infrastructure architecture](../../admin/infrastructure/architecture.md)

You should already know the basics of:

- Go packages, interfaces, table tests, and `t.Parallel()`.
- TypeScript, React, and React Query.
- SQL and REST APIs.
- Git branches and pull requests.
- Docker and Terraform concepts.

Run commands from the repository root unless a step says otherwise.

```sh
# Install repository git hooks.
git config core.hooksPath scripts/githooks

# Start the backend, frontend, PostgreSQL, and default Docker template.
./scripts/develop.sh
```

> [!NOTE]
> The development server creates default users and a Docker template. See
> [Contributing](./CONTRIBUTING.md#running-coder-on-development-mode) for the
> current credentials and workflow details.

## One-day schedule

| Time        | Block                        | Outcome                                     |
|-------------|------------------------------|---------------------------------------------|
| 09:00-09:30 | Orientation and setup        | Local server runs, hooks installed          |
| 09:30-10:15 | Architecture mental model    | You can explain the main runtime components |
| 10:15-11:30 | Backend API and database     | You can trace an API route to data access   |
| 11:30-12:15 | Frontend data flow           | You can trace a page to API queries         |
| 13:15-14:00 | Provisioning, agent, Tailnet | You can place code in the right subsystem   |
| 14:00-14:45 | Testing and quality gates    | You can choose focused checks for a change  |
| 14:45-16:30 | Full-stack capstone          | You build a disposable training patch       |
| 16:30-17:00 | Review and next steps        | You can write a PR summary and test plan    |

Adjust times as needed, but keep the capstone last. The earlier blocks teach the
code paths the capstone uses.

## Course modules

### 1. Orientation and setup

**Goal:** Get the project running and learn where important code lives.

Inspect these files and directories:

- `README.md`, product overview and quickstart.
- `AGENTS.md`, repository rules and required commands.
- `cmd/coder/main.go`, main binary entry point.
- `cli/`, user-facing command implementation.
- `coderd/`, main backend control plane.
- `site/`, frontend application.

Lab:

1. Start `./scripts/develop.sh`.
2. Open the dashboard from the development script output.
3. Create a local branch named `training/codebase-course`.
4. Write down the top-level directories that own CLI, backend, frontend,
   provisioning, agent runtime, networking, and enterprise features.

Stop when you can explain why `cmd/coder/main.go` is small and why most command
behavior lives in `cli/`.

### 2. Architecture mental model

**Goal:** Understand Coder as a control plane for cloud development
environments.

Core components:

| Component     | Code area                           | Responsibility                              |
|---------------|-------------------------------------|---------------------------------------------|
| CLI           | `cmd/coder/`, `cli/`                | Starts the server and calls Coder APIs      |
| Control plane | `coderd/`                           | API, auth, workspaces, templates, jobs      |
| SDK           | `codersdk/`                         | Shared API types and Go client methods      |
| Database      | `coderd/database/`                  | PostgreSQL schema, SQLC queries, authz      |
| Provisioner   | `provisioner/`, `provisionerd/`     | Runs Terraform and reports build results    |
| Agent         | `agent/`, `coderd/agentapi/`        | Runs inside workspaces and exposes services |
| Networking    | `tailnet/`, `coderd/workspaceapps/` | Connects users to agents and workspace apps |
| Frontend      | `site/`                             | React dashboard and browser workflows       |
| Enterprise    | `enterprise/`                       | Premium features and enterprise overrides   |

Lab:

1. Pick the workflow "create a workspace".
2. Trace it at a high level through frontend or CLI, `coderd`, database,
   provisioner, agent, and Tailnet.
3. Note where data is persisted and where runtime connectivity starts.

Stop when you can describe the difference between a workspace build and a
workspace agent.

### 3. Backend API and database trace

**Goal:** Learn how HTTP requests enter `coderd` and reach data access.

Inspect these areas:

- `coderd/coderd.go`, route registration and middleware wiring.
- `coderd/httpapi/`, response helpers and API utilities.
- `coderd/httpmw/`, request middleware and context loading.
- `codersdk/`, API types and Go client methods.
- `coderd/database/queries/`, SQLC query definitions.
- `coderd/database/dbauthz/`, database authorization wrappers.
- `coderd/database/db2sdk/`, database-to-SDK conversions.

Lab:

1. In `coderd/coderd.go`, find a route under `/api/v2` for a familiar resource,
   such as workspaces, templates, users, or organizations.
2. Trace the route to its handler.
3. Trace one database call from the handler to a query in
   `coderd/database/queries/`.
4. Identify whether the query uses the authenticated request context, a system
   context, or another authorization path.
5. Find the closest test for that handler or package.

Stop when you can explain which file you would edit for each part of a small
backend API change: SDK type, route, handler, SQL query, authorization, test, and
generated code.

> [!WARNING]
> Do not bypass `dbauthz` in real changes. If a public endpoint needs system
> access, follow existing restricted system-context patterns and add tests that
> prove unauthorized users cannot read protected data.

### 4. Frontend data flow trace

**Goal:** Learn how the React frontend gets data from the backend.

Inspect these areas:

- `site/src/App.tsx`, route definitions.
- `site/src/pages/`, page-level components.
- `site/src/modules/`, product-specific UI logic.
- `site/src/components/`, shared UI primitives.
- `site/src/api/api.ts`, API functions.
- `site/src/api/queries/`, React Query hooks and keys.
- `site/src/api/typesGenerated.ts`, generated API types.
- `site/src/testHelpers/`, frontend test helpers.

Lab:

1. Pick a visible dashboard page, such as workspaces, templates, users, health,
   or deployment settings.
2. Trace from `site/src/App.tsx` to the page component.
3. Find a query hook used by the page or one of its child components.
4. Trace the query to the API function in `site/src/api/api.ts`.
5. Match the API function to a backend route in `coderd/coderd.go`.

Stop when you can explain why components should use React Query instead of
calling `API` functions directly.

> [!NOTE]
> New frontend work should prefer shared components, Tailwind styling, generated
> API types, and Storybook coverage. Avoid adding new MUI or Emotion patterns.

### 5. Provisioning, agents, and networking

**Goal:** Know where workspace lifecycle code belongs.

Inspect these areas:

- `provisioner/terraform/`, Terraform execution and parsing.
- `provisionerd/`, provisioner daemon runtime.
- `provisionersdk/`, provisioner protocol and shared types.
- `coderd/provisionerdserver/`, control plane side of provisioner RPCs.
- `agent/`, workspace agent runtime.
- `coderd/agentapi/`, APIs consumed by workspace agents.
- `coderd/workspaceapps/`, browser access to workspace apps.
- `tailnet/`, peer coordination, DERP maps, tunnels, and connectivity.

Lab:

1. Trace a workspace build test or template-version test until it reaches a
   provisioner job.
2. Find where provisioner logs or diagnostics are reported back to `coderd`.
3. Find one agent-facing API under `coderd/agentapi/`.
4. Find one Tailnet or workspace app test that verifies connectivity behavior.

Stop when you can decide whether a change belongs in `coderd`, `provisioner`,
`provisionerd`, `agent`, `tailnet`, or `site`.

### 6. Testing and quality gates

**Goal:** Pick the smallest useful checks for a change, then know when broader
checks are required.

Important helpers and conventions:

- Backend tests should use `t.Parallel()` unless there is a clear reason not to.
- Use unique identifiers in concurrent tests.
- Use `coderd/coderdtest/` for API-server test setup.
- Use `testutil/` for common test helpers.
- Use `enterprise/coderd/coderdenttest/` for enterprise API tests.
- Use Storybook for UI component and page behavior coverage when the DOM is
  involved.
- Use focused tests before full suite targets.

Common commands:

```sh
# Focused Go test by name.
make test RUN=TestName

# Generate SQLC, OpenAPI, golden files, and related generated assets.
make gen

# Format and lint everything affected by the change.
make fmt
make lint

# Run the same checks that hooks use before committing.
make pre-commit
```

Frontend commands from `site/`:

```sh
pnpm test
pnpm test:storybook
pnpm lint
pnpm format
```

Lab:

1. Pick one traced backend handler and find the nearest focused test command.
2. Pick one traced frontend page or component and find the nearest Storybook,
   Vitest, or typecheck command.
3. Write down which checks you would run for a backend-only change, a
   frontend-only change, a SQL change, and a full-stack change.

Stop when you can explain why `make gen` is required after SQL or API generation
changes, and why hooks must not be skipped.

## Full-stack capstone

Build a disposable training patch that adds a small read-only codebase summary
surface. The goal is to practice the path, not to ship this exact feature.

### Capstone prompt

Add a local-only training page or temporary panel that shows a codebase summary
from a new backend endpoint. The summary should include a short list of core
components, their code paths, and their responsibilities.

Example response shape:

```json
{
  "components": [
    {
      "name": "coderd",
      "path": "coderd/",
      "responsibility": "Main API control plane"
    },
    {
      "name": "site",
      "path": "site/",
      "responsibility": "React dashboard"
    }
  ]
}
```

### Required backend work

1. Add an SDK response type in a new or existing `codersdk/` file.
2. Add a `codersdk.Client` method for the endpoint.
3. Add a backend handler in `coderd/` that returns the summary.
4. Register the route under `/api/v2` in `coderd/coderd.go`.
5. Add a focused `coderd` test using existing `coderdtest` patterns.
6. For a real PR, add Swagger annotations and run `make gen`.

### Required frontend work

1. Add an API function in `site/src/api/api.ts` or a nearby API file that matches
   existing patterns.
2. Add a React Query hook and key under `site/src/api/queries/`.
3. Add a temporary page, panel, or component that renders loading, success, and
   error states.
4. Use generated API types when available. For a disposable patch, document any
   temporary type you add and remove it before turning the work into a real PR.
5. Add Storybook coverage for the display component, including loading and error
   states if the component owns those states.

### Required review work

1. Write a short PR-style summary.
2. List every check you ran.
3. Explain why the patch is disposable and what would need to change before it
   could become a production feature.
4. Reset the branch when the exercise is complete, or keep it only as a private
   training branch.

### Suggested capstone checks

Run the narrowest useful checks first:

```sh
# Replace TestName with the test you added.
make test RUN=TestName

# Run after generated API, SQL, or OpenAPI changes.
make gen
```

From `site/`:

```sh
pnpm test:storybook path/to/your.stories.tsx
pnpm lint:types
pnpm format
```

Before turning the exercise into a real PR, run the broader repository checks
that apply to the files you changed.

## Validation checklist

Use this checklist at the end of the day:

- [ ] I can explain the request path from `site/` to `coderd` to the database.
- [ ] I can find a route in `coderd/coderd.go` and trace it to a handler.
- [ ] I can find the SDK type or client method used by a frontend or CLI flow.
- [ ] I can identify the SQLC query and dbauthz path used by a backend handler.
- [ ] I can explain what the provisioner, agent, and Tailnet each own.
- [ ] I can choose focused backend and frontend tests for a small change.
- [ ] I know when to run `make gen`, `make fmt`, `make lint`, and
      `make pre-commit`.
- [ ] I can write a PR summary, test plan, and rollback note for a small change.

## What to learn later

These topics are important, but they are intentionally outside this one-day
fundamentals course:

- Deep Tailnet and DERP internals.
- Workspace proxy internals.
- OAuth2 provider internals.
- Advanced RBAC policy authoring.
- High availability and clustering.
- Scale testing.
- License enforcement internals.
- Chat and AI feature internals.
- Deprecated MUI and Emotion authoring patterns.

Learn these when your first real issue requires them.

## Related documentation

- [Contributing](./CONTRIBUTING.md)
- [Backend](./backend.md)
- [Frontend](./frontend.md)
- [Documentation](./documentation.md)
- [Templates](./templates.md)
- [Infrastructure architecture](../../admin/infrastructure/architecture.md)
- [Networking](../../admin/networking/index.md)
- [Provisioners](../../admin/provisioners/index.md)
