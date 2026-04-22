# Agent UI Plugin System — Implementation Summary

## What was built

A plugin system for the Coder agents UI (`/agents`) that allows UI extensions to be rendered as iframe tabs in the right panel alongside Git, Terminal, and Desktop.

## Architecture

```
Terraform template (future)
  └─ coder_agent_plugin resource
       └─ provisioner proto ──► coderd DB ──► agent manifest
                                    │
                                    ▼
                              REST API (plugins in WorkspaceAgent response)
                                    │
                                    ▼
                              Frontend (AgentChatPageView)
                                    │
                                    ▼
                              PluginIframe component
                                    │
                                    ▼
                              Sandboxed iframe ◄──► postMessage SDK
                                    │
                                    ▼
                              Plugin code (served from S3-compatible URL)
```

## Components

### Database (Phase 1)

| File | Purpose |
|------|---------|
| `coderd/database/migrations/000472_workspace_agent_plugins.up.sql` | Creates `workspace_agent_plugins` table with `(agent_id, slug)` unique constraint |
| `coderd/database/migrations/000473_add_echo_chat_provider.up.sql` | Adds "echo" to the `chat_providers` provider check constraint |
| `coderd/database/queries/workspaceagentplugins.sql` | SQLC queries: Upsert, GetByAgentID, GetByAgentIDs, GetByAgentIDAndSlug |
| `coderd/database/migrations/testdata/fixtures/000472_workspace_agent_plugins.up.sql` | Test fixture |

**Table schema:**
```sql
workspace_agent_plugins (
    id            UUID PRIMARY KEY,
    created_at    TIMESTAMP WITH TIME ZONE NOT NULL,
    agent_id      UUID NOT NULL REFERENCES workspace_agents(id) ON DELETE CASCADE,
    slug          VARCHAR(64) NOT NULL,
    display_name  VARCHAR(256) NOT NULL DEFAULT '',
    icon          VARCHAR(256) NOT NULL DEFAULT '',
    url           VARCHAR(4096) NOT NULL,        -- S3-compatible base URL
    backend_entry VARCHAR(1024) NOT NULL DEFAULT '', -- future: backend JS entry point
    UNIQUE (agent_id, slug)
)
```

### Proto (Phase 2)

| File | Change |
|------|--------|
| `provisionersdk/proto/provisioner.proto` | Added `Plugin` message, `repeated Plugin plugins = 27` on `Agent` |
| `agent/proto/agent.proto` | Added `WorkspaceAgentPlugin` message, `repeated WorkspaceAgentPlugin plugins = 20` on `Manifest` |

### Backend Go (Phase 3)

| File | Purpose |
|------|---------|
| `codersdk/workspaceagentplugins.go` | SDK type `WorkspaceAgentPlugin` |
| `codersdk/workspaceagents.go` | Added `Plugins` field to `WorkspaceAgent` struct |
| `coderd/database/db2sdk/db2sdk.go` | `WorkspaceAgentPlugins()` and `WorkspaceAgentPlugin()` converters; updated `WorkspaceAgent()` signature to accept plugins |
| `coderd/database/dbauthz/dbauthz.go` | Authorization for all 4 plugin query methods (follows workspace app pattern) |
| `coderd/provisionerdserver/provisionerdserver.go` | `insertAgentPlugin()` — inserts plugins during workspace provisioning |
| `coderd/agentapi/manifest.go` | Fetches plugins in parallel, converts to proto, includes in agent manifest |
| `coderd/workspaceagents.go` | Main agent handler fetches and includes plugins |
| `coderd/workspacebuilds.go` | Workspace builds data loader fetches plugins; passes through to SDK conversion |
| `coderd/workspaces.go` | Passes plugins through workspace build conversion |

**API surface:** Plugins are included in every `WorkspaceAgent` response — no new endpoints needed. Any API that returns workspace agent data (direct agent lookup, workspace builds, workspace list) now includes the `plugins` array.

### Frontend (Phase 4)

| File | Purpose |
|------|---------|
| `site/src/pages/AgentsPage/components/RightPanel/pluginMessageBus.ts` | Typed postMessage protocol definitions (`coder-plugin:init/ready/port-request/port-response/token-refresh`) and `PluginContext` type |
| `site/src/pages/AgentsPage/components/RightPanel/PluginIframe.tsx` | Sandboxed iframe component with activation gate, loading state, postMessage handshake, token refresh |
| `site/src/pages/AgentsPage/hooks/usePluginToken.ts` | Hook that mints a 1-hour scoped API token via `POST /api/v2/users/me/keys/tokens`, auto-refreshes before expiry |
| `site/src/pages/AgentsPage/AgentChatPageView.tsx` | Plugin tabs integrated into RightPanel `SidebarTabView` alongside Git/Terminal/Desktop |

**Plugin lifecycle:**
1. Plugin buttons appear in RightPanel tab bar (one per `workspaceAgent.plugins[]` entry)
2. User clicks tab → iframe mounts, token minted
3. iframe loads plugin URL → host sends `coder-plugin:init` with token + context
4. Plugin sends `coder-plugin:ready` → loading spinner removed
5. Plugin can request port URLs via `coder-plugin:port-request`
6. Token auto-refreshes via `coder-plugin:token-refresh`
7. iframe stays alive when switching tabs (hidden/inert pattern)

**iframe sandbox:** `allow-scripts allow-forms allow-same-origin` — `allow-same-origin` is required because the Coder port forwarding proxy needs cookie-based auth.

### Echo Test Provider

| File | Purpose |
|------|---------|
| `coderd/x/chatd/chatprovider/echomodel/echomodel.go` | Implements `fantasy.LanguageModel` — drives workspace creation via tool calls, then echoes text |
| `coderd/x/chatd/chatprovider/chatprovider.go` | Registers "echo" as a supported provider, skips API key requirement |

**Echo model behavior:**
1. First call with tools → calls `list_templates`
2. Gets template list → calls `create_workspace` with first template
3. Workspace created → calls `start_workspace`
4. Workspace ready → responds with text pointing user to plugin tabs

### Dev Server Auto-Setup

| File | Purpose |
|------|---------|
| `scripts/develop/main.go` | `setupEchoProvider()` — creates echo provider + model config via API on startup |
| `scripts/develop/main.go` | Static file server for `examples/plugins/demo/` on port 9876 |
| `scripts/develop/main.go` | `pollAndInsertDemoPlugins()` — background goroutine that inserts demo plugin row for every workspace agent every 10 seconds |
| `scripts/develop/main.go` | Startup banner includes Demo Plugin URLs |

### Demo Plugin

| File | Purpose |
|------|---------|
| `examples/plugins/demo/index.html` | Entry point with context display, API test, port request, message log sections |
| `examples/plugins/demo/plugin.js` | Full SDK exercise: init handshake, API calls with token, port requests, token refresh, live message log |
| `examples/plugins/demo/style.css` | Dark theme matching Coder UI |

## What's not yet done

| Item | Status |
|------|--------|
| `terraform-provider-coder` resource (`coder_agent_plugin`) | Deferred — separate repo |
| Backend plugin execution in workspace | Future phase |
| MCP server registration from plugins | Future phase |
| S3 authentication for private buckets | Future phase |
| Plugin versioning | Future phase |
| Token scope narrowing (currently `coder:all`) | Future iteration |
| Narrowing `allow-same-origin` sandbox | Needs investigation into proxy auth alternatives |

## Testing the implementation

1. Start dev server: `./scripts/develop.sh`
2. Login at the Web UI (`admin@coder.com` / `SomeSecurePassword!`)
3. Go to `/agents`, select **Echo (Test)** model
4. Send any message — echo model creates a workspace via tool calls
5. Wait for workspace to build (~1 min) and agent to connect
6. Background poller inserts demo plugin within 10 seconds
7. **Demo Plugin** tab appears in right panel
8. Click to activate — iframe loads, handshake completes, plugin shows context

## Branch

`scott/agent-ui-plugins` on `github.com/coder/coder`

## Commit history

| Commit | Description |
|--------|-------------|
| `5677e0e` | feat: add agent UI plugin system (database, proto, SDK, API, frontend, demo) |
| `fe02996` | feat: add echo test provider and dev auto-setup |
| `54a65af` | fix: register echo provider in NormalizeProvider and pass API key |
| `1f7f294` | fix: pass dummy API key for echo provider setup |
| `952c71e` | feat: add echo provider DB migration |
| `b18b2c1` | feat: echo model calls tools to create workspace |
| `e610ff6` | feat: auto-insert demo plugin for all workspace agents |
| `ec42272` | fix: include plugins in workspace build API responses |
| `04ce8b5` | fix: retry plugin init when token arrives after iframe load |
| `19582bd` | fix: derive plugin URL from CODER_URL for correct port forwarding pattern |
| `c4b8ea7` | fix: add allow-same-origin to plugin iframe sandbox |
