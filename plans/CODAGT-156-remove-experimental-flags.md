# CODAGT-156: Remove experimental flags from Coder Agents

## Goal

Remove the `ExperimentAgents` feature flag so the Agents feature is always
available (backend routes + UI). Do **not** move any API routes (keep
`/api/experimental/` paths). Add a "BETA" badge next to the Coder logo in
the Agents page sidebar.

---

## Changes

### 1. Backend: Remove experiment middleware from routes

**File:** `coderd/coderd.go`

- **Lines ~1157-1159** — Remove `httpmw.RequireExperimentWithDevBypass(api.Experiments, codersdk.ExperimentAgents)` from the `/api/experimental/chats` route group. Keep `apiKeyMiddleware`.
- **Lines ~1267** — Remove the same middleware from `/api/experimental/mcp/servers` route group.
- **Lines ~1993-1999** — Remove the `ExperimentAgents` conditional around the `blob:` CSP `img-src` addition; make it unconditional.

### 2. Backend: Always include AgentsAccessRole

**File:** `coderd/roles.go`

- **Lines ~51-53** — Remove the `if api.Experiments.Enabled(codersdk.ExperimentAgents) || buildinfo.IsDev()` guard. Always append `rbac.AgentsAccessRole()` to site roles.

### 3. Backend: Always set AgentsTabVisible

**File:** `site/site.go`

- **Lines ~528-535** — Remove the experiment check. Always set `agentsTabVisible = true`.

### 4. SDK: Remove ExperimentAgents entirely

**File:** `codersdk/deployment.go`

- Remove the `ExperimentAgents` constant (~line 4397).
- Remove `ExperimentAgents` from `ExperimentsKnown` list (~line 4435).
- Remove the `case ExperimentAgents:` from the `DisplayName()` switch (~line 4414).
- Remove the TODO comment about adding it to `ExperimentsSafe` (~line 4444).

Existing deployments that still pass `--experiments=agents` will get a
"ignoring unknown experiment" warning log on startup, which is fine.

### 5. Frontend: Remove experiment check in Navbar

**File:** `site/src/modules/dashboard/Navbar/NavbarView.tsx`

- **Lines ~263-281** — In `AgentsNavItem`, remove the `experimentEnabled` check (`experiments.includes("agents") || isDevBuild(buildInfo)`). The nav item should render whenever `canCreateChat` is true. Remove the `useDashboard()` call and `buildInfo`/`experiments` destructuring if no longer needed.

### 6. Frontend: Remove experiment check in WorkspacesPage

**File:** `site/src/pages/WorkspacesPage/WorkspacesPage.tsx`

- **Line ~73** — Remove `const agentsEnabled = experiments.includes("agents");`. Change the query `enabled` condition (~line 137) to no longer depend on this flag (use `true` or just the `workspaceIds.length > 0` condition).

### 7. Frontend: Remove experiment from Storybook story

**File:** `site/src/modules/dashboard/Navbar/NavbarView.stories.tsx`

- **Line ~23** — Remove `experiments: ["agents"]` from the story mock since the experiment is no longer needed.

### 8. Frontend: Add "BETA" badge next to Coder logo

**File:** `site/src/pages/AgentsPage/components/Sidebar/AgentsSidebar.tsx`

- **Lines ~992-998** — After the `<NavLink>` containing the logo, add a `<FeatureStageBadge contentType="beta" size="sm" />` inline with the logo. Wrap logo + badge in a flex row with `gap-2 items-center` if not already.

**File:** `site/src/pages/AgentsPage/components/AgentPageHeader.tsx`

- **Lines ~46-52** — Same change for the mobile logo: add the beta badge next to the logo `<NavLink>`.

Import: `import { FeatureStageBadge } from "#/components/FeatureStageBadge/FeatureStageBadge";`

### 9. Documentation: Remove experiment setup instructions

**File:** `docs/ai-coder/agents/early-access.md`

- **Lines ~46-56** — Remove the "Enable Coder Agents" section that tells users to pass `--experiments=agents`.

**File:** `docs/ai-coder/agents/getting-started.md`

- **Lines ~33-42** — Remove "Step 1: Enable the experiment" section. Renumber subsequent steps.

**File:** `docs/ai-coder/agents/chats-api.md`

- **Line ~4** — Remove the statement that the API is "gated behind the `agents` experiment flag".

### 10. Backend: Clean up stale comments

**File:** `coderd/coderd.go`

- **Line ~1155** — Remove or reword the comment `// Experimental(agents): chat API routes gated by ExperimentAgents.`
- **Lines ~1993-1994** — Update the comment about `blob:` CSP to remove the experiment reference (the addition is now unconditional).

### 11. Tests: Remove experiment setup from tests

All these tests set `ExperimentAgents` in deployment values. Remove the experiment assignment lines — the feature is now unconditional.

| File                                  | Lines                       |
|---------------------------------------|-----------------------------|
| `coderd/x/chatd/integration_test.go`  | ~40, 301, 456               |
| `coderd/x/chatd/chatd_test.go`        | ~197, 362, 3041, 3222, 6033 |
| `coderd/mcp_test.go`                  | ~28                         |
| `coderd/exp_chats_test.go`            | ~54                         |
| `enterprise/coderd/roles_test.go`     | ~456                        |
| `enterprise/coderd/exp_chats_test.go` | ~1107, 1185                 |

### 12. Generated types

**File:** `site/src/api/typesGenerated.ts`

- Run `make gen` to regenerate. The `"agents"` value will be removed from the `Experiment` type once it's out of `ExperimentsKnown`.

---

## Verification

1. `make gen` — Regenerate types and ensure no drift.
2. `make fmt` — Format all code.
3. `make lint` — Ensure no lint errors.
4. `make build` — Ensure clean build.
5. Targeted tests:
   - `make test RUN=TestAssignableRoles` (roles always include agents)
   - `make test RUN=TestChat` (chat routes accessible without experiment)
   - `make test RUN=TestMCP` (MCP routes accessible without experiment)

---

## Out of scope

- Moving API routes from `/api/experimental/` to another path.
- Changes to the `RequireExperiment` / `RequireExperimentWithDevBypass` middleware functions themselves.
