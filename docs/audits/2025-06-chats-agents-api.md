# Chats / Agents API Audit

Self-audit of the `/chats` and `/agents` API surface, covering chat
authorization, agent connection and multi-agent scenarios, workspace
sharing, prebuilds, and devcontainer interactions.

An initial pass produced 18 findings. A skeptical re-verification against
the actual code found that **5 were wrong**, **3 were design choices
mischaracterized as bugs**, and **6 were significantly overstated**. This
document contains only the findings that survived both passes.

---

## Confirmed findings

### 1. BUG — Missing `return` after error in `workspaceAgentReinit`

**Severity:** High
**File:** `coderd/workspaceagents.go:1480–1494`

When `GetWorkspaceByAgentID` fails, `httpapi.InternalServerError` writes
an HTTP 500 but the handler does not return. Execution falls through:
`workspace` is zero-valued so `workspace.ID` is `uuid.Nil`. The handler
then calls `ListenForWorkspaceClaims` with a bogus channel ID and
attempts to create an SSE transmitter on a response that already has a
500 status.

The very next error block (line 1492–1493) correctly includes `return`,
making this a clear copy-paste omission.

**Impact:** Panic risk on agent reinit failures. Potential double-write
to `ResponseWriter`. Bogus pubsub subscription on `uuid.Nil` channel.

**Fix:** Add `return` after line 1483.

---

### 2. CODE QUALITY — `watchChatGit` inconsistent with `watchChatDesktop` on workspace authz

**Severity:** Low (defense-in-depth)
**File:** `coderd/exp_chats.go:1192–1265`

`watchChatDesktop` (lines 1354–1365) checks `ActionApplicationConnect`
and `ActionSSH` on the workspace before dialing the agent. `watchChatGit`
skips this and goes directly to
`GetWorkspaceAgentsInLatestBuildByWorkspaceID`.

Not a security vulnerability today because `ExtractChatParam` middleware
enforces `ActionRead` on the owner-scoped chat resource, which
implicitly guarantees workspace ownership. But if chat ownership
semantics ever change (e.g. shared team chats), this becomes a real
hole.

**Fix:** Copy the 5-line authz check from `watchChatDesktop`.

---

### 3. EDGE CASE — No `ActionSSH` re-check on chat message send

**Severity:** Low (stale authorization)
**File:** `coderd/exp_chats.go:1544–1642`

`postChatMessages` does not re-validate that the user still has
`ActionSSH` on the chat's workspace. The middleware only checks
`ActionRead` on the chat resource (owner-scoped). If workspace SSH
access is revoked after chat creation, the user can continue sending
messages and the daemon continues using the workspace.

Mitigating factors:

- Chat RBAC ensures only the chat owner can send messages.
- The scenario requires a user to create a chat, have SSH access
  revoked, then continue using the existing chat—a narrow
  privilege-revocation gap.

---

### 4. LIMITATION — Hardcoded `agents[0]` in all chat code paths

**Severity:** Low (feature limitation)
**Files:** `exp_chats.go:1224,1385`, `chatd.go:217`,
`createworkspace.go:210,315,338,348,354`, `startworkspace.go:171`

Every chat code path selects the first agent from
`GetWorkspaceAgentsInLatestBuildByWorkspaceID` without filtering or
sorting. There is no logic to select by name, devcontainer association,
or to handle multi-agent templates.

Not a bug today—multi-agent chat is not a claimed feature and most
templates define a single agent. But as the chat feature matures toward
devcontainer support this will need to be addressed. The SQL query also
has no `ORDER BY`, so agent selection is not even deterministic.

---

### 5. RESOURCE EXHAUSTION — No chat-specific rate limiting

**Severity:** Medium
**File:** `coderd/coderd.go:1153`

Chat endpoints only get the global `apiRateLimiter` (512/min). There is
no chat-specific rate limit on `POST /chats/{chat}/messages`. An
authenticated user could rapidly send messages to trigger many
concurrent LLM API calls. The spend-based usage limit provides cost
control but not rate control. `MaxQueueSize` limits queued messages per
chat, but a user with many chats could generate significant load.

---

### 6. TESTABILITY — `quartz.NewReal()` hardcoded in production chatd tool creation

**Severity:** Low
**File:** `coderd/x/chatd/chatd.go:3501`

The computer-use tool is created with `quartz.NewReal()` hardcoded
instead of using `p.clock` from the `Config`. The rest of chatd properly
uses the injected quartz clock.

---

### 7. TESTABILITY — `time.NewTicker` / `time.AfterFunc` instead of quartz

**Severity:** Low
**Files:** `coderd/x/chatd/chattool/createworkspace.go:383,445,475`,
`coderd/x/chatd/chatloop.go:579`

Three uses of `time.NewTicker` and one `time.AfterFunc` bypass the
injected quartz clock, making these code paths untestable with fake
clocks. Tests work around it by making polled operations return
immediately.

---

## Retracted findings

The following findings from the initial audit pass were wrong or
significantly overstated. They are documented here to explain why they
were dropped.

### ~~Chat workspace creation bypasses prebuilds~~ — Wrong

The initial audit read `createworkspace.go:161–164` in isolation and
concluded that `TemplateVersionPresetID` is never set. This missed the
handler-side fallback in `coderd/workspaces.go:691–710` where
`FindMatchingPresetID` auto-resolves a matching preset when none is
provided. A second fallback exists in `wsbuilder.go:492–498`. The chat
tool supplies parameters via `RichParameterValues`, so if they match a
preset a prebuild is claimed through the same path as any non-preset
workspace creation.

### ~~No unique constraint on `chats.workspace_id`~~ — Wrong (intentional design)

Multiple chats sharing a workspace is a core feature. The
subagent/delegation architecture in `coderd/x/chatd/subagent.go:355–404`
creates child chats with `parent.WorkspaceID`—they intentionally share
the workspace. A `UNIQUE` constraint would break this. The chat creation
API also allows explicitly attaching a chat to an existing workspace.

### ~~Chat creation bypasses workspace sharing org gate~~ — Wrong

The initial audit confused "using a workspace" with "sharing a
workspace." `ShareableWorkspaceOwners` controls who can modify ACLs
(enforced only in `patchWorkspaceACL` and `deleteWorkspaceACL`).
`ActionSSH` controls who can use a workspace. The chat path checks
`ActionSSH`, which is correct. Creating a chat does not share the
workspace with anyone.

### ~~Subagent chats inherit parent's stale `workspace_id`~~ — Wrong

The initial audit cited `coderd/agentapi/subagent.go:315,385`—the
workspace agent subagent API (devcontainer infrastructure), which has
zero references to chats. Chat subagents live in
`coderd/x/chatd/subagent.go`, where `workspace_id` is refreshed from
the database at spawn time and the parent workspace must be running.

### ~~Admins cannot list other users' chats~~ — Design choice

Test comments explicitly document this: "Org admin should NOT see other
users' chats—chats are not org-scoped resources." Admins access
individual chats by ID, which is fully RBAC-authorized.

### ~~No chat deletion endpoint~~ — Design choice

Archiving is deletion. `patchChat` with `Archived: true` calls
`chatDaemon.ArchiveChat()` which broadcasts `ChatEventKindDeleted`.
`ActionDelete` in RBAC policy is a forward-compatibility stub.

### ~~No proactive notification on workspace stop~~ — Design choice

Lazy discovery is the deliberate model. Tool calls that need the
workspace fail fast, and `checkExistingWorkspace` handles the full
spectrum of workspace states. Proactive notification would add
complexity for marginal benefit.

### ~~Workspace deletion leaves chats dangling~~ — Overstated

Both deletion paths are handled at the tool level. Soft-delete:
`checkExistingWorkspace` detects `ws.Deleted` and allows new workspace
creation. Hard-delete: `ON DELETE SET NULL` fires and
`loadWorkspaceAgentLocked` detects the invalid workspace ID. The gap is
one failed tool call before self-correction.

### ~~Concurrent agent connection state race~~ — Overstated

The tailnet coordinator in `tailnet/coordinator.go:424–458` explicitly
handles duplicate peer IDs by closing the old connection with
`CloseErrOverwritten`. The state self-heals within one ping period.

### ~~No subagent cleanup on parent disconnect~~ — Overstated

Agent-side cleanup on startup (`agentcontainers/api.go:614–631`) and
graceful shutdown (`api.go:2173–2210`) exist. The gap is only for
ungraceful crashes without rebuild, and it self-heals on reconnect.

### ~~`DeleteSubAgent` no parent ownership check~~ — Overstated

No discovery vector exists (`ListSubAgents` is parent-scoped), subagent
IDs are random v4 UUIDs, and dbauthz provides workspace-level
authorization. Defense-in-depth gap but not exploitable.

### ~~Non-atomic `patchChat` updates~~ — Overstated

Both fields are independently meaningful. A partial update is not
corrupting—the client gets an error and can retry.

### ~~Build failure orphans workspace~~ — Overstated

Factually correct but TTL, dormancy detection, and auto-deletion provide
cleanup. Consistent with platform-wide behavior for failed builds.

---

## Methodology note

The initial audit ran three parallel agents covering (1) chat API
handlers and authorization, (2) agent connections and multi-agent
scenarios, and (3) workspace sharing, prebuilds, and devcontainers. A
verification agent confirmed line numbers for the top findings. A second
pass then ran four skeptical agents tasked with disproving each finding,
checking for middleware, handler fallbacks, intentional design, and
mitigating mechanisms the first pass missed.
