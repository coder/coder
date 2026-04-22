# Agent UI Plugin System — Production Readiness Plan

## Current State

The plugin system is functionally complete for the frontend-only path: plugins defined in the DB render as iframe tabs in the agents UI RightPanel, communicate via a typed postMessage SDK, and receive short-lived API tokens. An echo test provider and demo plugin exist for development testing.

**Not production-ready.** The audit identified security gaps, missing tests, frontend robustness issues, and incomplete API integration. This document covers what needs to happen before this ships.

---

## Phase 1: Security Hardening

### 1.1 Token Scope Narrowing

**Current:** Plugins receive a `scope: "all"` token — full API access.

**Required:** Least-privilege token. A plugin should only be able to:
- Read workspace/agent info for its own workspace
- Access port forwarding URLs
- Read build info

**Work:**
- Define a new composite scope (e.g., `coder:plugins`) in `coderd/rbac/scopes_catalog.go` that grants `workspace:read` + `api_key:read`
- Update `usePluginToken.ts` to request this scope
- Test that the demo plugin's API calls still work with the reduced scope

### 1.2 postMessage Origin Validation

**Current:** The host accepts messages from any origin with the `coder-plugin:` prefix.

**Required:** Validate `event.origin` in the message handler.

**Work:**
- In `PluginIframe.tsx`, check `event.origin` against the plugin's URL origin
- When sending messages to the iframe, use the plugin's origin instead of `"*"`

### 1.3 Plugin URL Validation

**Current:** No validation of the `url` field in plugin definitions.

**Required:** Reject `javascript:`, `data:`, and non-HTTPS URLs (with an exception for `http://localhost` in dev mode).

**Work:**
- Add URL validation in `insertAgentPlugin()` in `provisionerdserver.go`
- Reject URLs that don't start with `https://` (or `http://localhost` for dev)

### 1.4 Token Revocation on Unmount

**Current:** Tokens are created but never revoked — they expire after 1 hour.

**Required:** Revoke the token when the plugin tab unmounts or the chat page closes.

**Work:**
- In `usePluginToken.ts`, store the token name and call `DELETE /api/v2/users/me/keys/{name}` on cleanup
- Handle cleanup failure gracefully (token still expires naturally)

---

## Phase 2: Test Coverage

### 2.1 Backend Tests

**Database queries** (`coderd/database/queries/workspaceagentplugins_test.go` — new):
- `TestUpsertWorkspaceAgentPlugin/Insert` — verify all fields stored
- `TestUpsertWorkspaceAgentPlugin/Upsert` — update existing plugin
- `TestGetWorkspaceAgentPluginsByAgentID/Empty` — returns empty slice
- `TestGetWorkspaceAgentPluginsByAgentID/Multiple` — returns sorted by slug

**API integration** (`coderd/workspaceagents_test.go` — add cases):
- `TestWorkspaceAgent_PluginsIncluded` — GET agent returns plugins array
- `TestWorkspaceAgent_PluginsEmpty` — returns `[]` not `null`

**Provisioner** (`coderd/provisionerdserver/provisionerdserver_test.go` — add cases):
- `TestCompleteJob_PluginInsertion` — provision with plugin proto results in DB row
- `TestCompleteJob_PluginUpsert` — re-provision updates existing plugin

**Workspace builds** (`coderd/workspacebuilds_test.go` — add cases):
- `TestWorkspaceBuild_PluginsInResponse` — build response includes plugin data

### 2.2 Frontend Tests

**Storybook stories** (`PluginIframe.stories.tsx` — new):
- `Default` — renders with mock plugin, shows loading state
- `Ready` — handshake complete, plugin visible
- `MultiplePlugins` — multiple tabs render correctly
- `NotActivated` — shows activation placeholder

**Hook test** (`usePluginToken.test.ts` — new):
- Token creation on activation
- No token before activation
- Cleanup on unmount

---

## Phase 3: Frontend Robustness

### 3.1 iframe Error Handling

**Current:** If the plugin URL is unreachable, the spinner shows forever.

**Work:**
- Add `onerror` handler on the iframe
- Add a timeout (e.g., 30 seconds) — if no `ready` received, show error with retry button
- Show the actual URL in the error message for debugging

### 3.2 Empty/Null Array Consistency

**Current:** Some API paths return `null` for plugins, others return `[]`.

**Work:**
- Ensure `db2sdk.WorkspaceAgentPlugins()` always returns `[]codersdk.WorkspaceAgentPlugin{}` (never nil)
- Frontend should defensively handle both (`workspaceAgent?.plugins ?? []` — already done)

### 3.3 Clarify nil Plugin Call Sites

Several internal endpoints pass `nil` for plugins in `db2sdk.WorkspaceAgent()`:
- `coderd/workspaceagents.go` lines ~708, 819, 942, 1035 (SSH, containers, port-share endpoints)
- `coderd/exp_chats.go` lines ~1742, 1888

**Decision needed:** Are these intentionally omitting plugins (internal/perf optimization) or bugs? Document the intent with comments if intentional. If they should include plugins, fetch and pass them.

**Recommendation:** These are internal endpoints that don't render UI, so `nil` is fine. Add a comment at each call site: `// Plugins omitted: not needed for this endpoint.`

---

## Phase 4: API & Documentation

### 4.1 Swagger Annotations

- Add `// @Description` comments to `WorkspaceAgentPlugin` struct fields in `codersdk/workspaceagentplugins.go`
- Verify generated swagger includes plugins in workspace agent response schema
- Run `make gen` to regenerate API docs

### 4.2 User Documentation

Create `docs/admin/plugins.md`:
- What plugins are and how they work
- How to define a plugin in a Terraform template (once provider support exists)
- Plugin URL requirements (HTTPS, serves index.html)
- Security model (sandboxed iframe, scoped token)
- postMessage SDK reference

Create `docs/contributing/plugins.md`:
- How to write a plugin (reference the demo plugin)
- Message bus protocol
- Available context (API URL, token, workspace/agent/chat IDs)
- Port URL requests

---

## Phase 5: Terraform Provider

**Repo:** `terraform-provider-coder` (separate PR)

Add `coder_agent_plugin` resource:
```hcl
resource "coder_agent_plugin" "monitor" {
  agent_id      = coder_agent.main.id
  slug          = "monitor"
  display_name  = "Monitor"
  icon          = "/icons/monitor.svg"
  url           = "https://s3.example.com/plugins/monitor/"
  backend_entry = ""  # reserved for future backend execution
}
```

**Work:**
- Add resource schema in the Terraform provider
- Map fields to the provisioner proto `Plugin` message
- Add acceptance tests
- Publish provider version

Until this ships, plugins can only be inserted via direct DB access or the provisioner proto.

---

## Phase 6: Cleanup & PR Preparation

### 6.1 Squash Commits

The branch has 11 commits with incremental fixes. Squash into logical commits:
1. `feat: add agent UI plugin system` — DB, proto, SDK, API, frontend
2. `feat: add echo test provider` — echomodel, chatprovider registration, DB migration
3. `feat: add dev server plugin auto-setup` — dev server changes, demo plugin

### 6.2 Remove Dev-Only Code from Production Path

- The echo provider and dev auto-setup code are fine (echo is a proper provider, dev setup is in `scripts/develop/`)
- Ensure the `pollAndInsertDemoPlugins` function is only in the dev script, not in coderd
- Confirm `.gitignore` includes the `develop` binary

### 6.3 Run Full CI

- `make pre-commit` — gen, fmt, lint, build
- `make test` — full test suite
- `make test-race` — race detector
- Storybook build

---

## Phase 7: Follow-Up Work (Post-Merge)

These are explicitly out of scope for the initial PR but should be tracked:

| Item | Description |
|------|-------------|
| Backend execution | Run plugin backend JS in the workspace via `backend_entry` field |
| MCP registration | Plugin backends register as MCP providers on activation |
| S3 auth | Support private S3 buckets with credentials in Terraform |
| Plugin versioning | URL-based versioning or manifest with version field |
| Plugin marketplace | Discovery/registry for community plugins |
| Scoped sandbox | Per-plugin `allow_same_origin` flag in Terraform |
| Plugin settings | Per-user plugin preferences/state persisted server-side |

---

## Ordering & Dependencies

```
Phase 1 (Security)  ──┐
Phase 2 (Tests)     ──┼──► Phase 6 (Cleanup & PR) ──► Merge
Phase 3 (Robustness)──┤
Phase 4 (Docs)      ──┘
Phase 5 (Terraform)  ──► Separate PR in terraform-provider-coder
Phase 7 (Follow-up)  ──► Tracked as issues
```

Phases 1–4 can be worked in parallel. Phase 6 depends on all of them. Phase 5 is independent (separate repo). Phase 7 is post-merge.

---

## Estimated Effort

| Phase | Effort | Notes |
|-------|--------|-------|
| Phase 1: Security | 1 day | Token scope is the biggest piece |
| Phase 2: Tests | 1-2 days | Backend tests are mechanical; frontend stories need care |
| Phase 3: Robustness | 0.5 day | Error handling + nil cleanup |
| Phase 4: Docs | 0.5 day | Swagger + user docs |
| Phase 5: Terraform | 1 day | Separate repo, can be parallel |
| Phase 6: Cleanup | 0.5 day | Squash, CI, final review |
| **Total** | **~5 days** | Phases 1-4 parallelizable |
