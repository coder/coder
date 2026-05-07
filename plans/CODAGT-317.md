# CODAGT-317: PR workspaces sometimes require name confirmation to delete

## Problem

The archive/delete flow shows a type-the-workspace-name confirmation dialog
for disposable PR workspaces when it shouldn't. The current heuristic compares
`workspace.created_at >= chat.created_at` to determine if a workspace was
auto-created by the chat. This produces false positives when:

- A chat is created against a pre-existing PR workspace
- Multiple chats share the same workspace
- A chat row is recreated/migrated after the workspace
- Clock skew between workspace and chat creation

## Approach

Add a `workspace_auto_created` boolean column to the `chats` table. Set it to
`true` only when the `create_workspace` chat tool provisions a new workspace.
The frontend reads this field to decide whether to skip the confirmation dialog,
replacing the fragile timestamp comparison.

This is option 3 from the thread discussion: the most correct fix that
eliminates all false positives.

## Implementation Plan

### 1. Database migration

- Create migration adding `workspace_auto_created BOOLEAN NOT NULL DEFAULT FALSE`
  to the `chats` table.
- Down migration drops the column.

### 2. SQL query changes

- Update `UpdateChatWorkspaceBinding` in `coderd/database/queries/chats.sql`
  to accept and set the new `workspace_auto_created` parameter.
- The `InsertChat` query already has the column via the migration default.

### 3. Run `make gen`

- Regenerate Go types, query methods, dbmem, dbmock, etc.
- Update `enterprise/audit/table.go` if the `Chat` type is audited.

### 4. Backend: set `workspace_auto_created = true` in `create_workspace` tool

- In `coderd/x/chatd/chattool/createworkspace.go`, when calling
  `UpdateChatWorkspaceBinding`, pass `workspace_auto_created = true`.
- In `coderd/x/chatd/chattool/startworkspace.go` (which binds an existing
  workspace), keep it `false` (default).

### 5. Backend: expose field in API response

- Verify the `Chat` Go struct and `codersdk.Chat` already pick up the new
  field via codegen. If not, add it to the SDK type and conversion.

### 6. Frontend: replace timestamp heuristic

- In `agentWorkspaceUtils.ts`:
  - Replace `isWorkspaceAutoCreated(workspaceCreatedAt, chatCreatedAt)` with
    a simple boolean check on `chat.workspace_auto_created`.
  - Simplify `resolveArchiveAndDeleteAction` to accept the boolean directly
    instead of fetching timestamps.
- Update `AgentsPage.tsx` call site to pass the new field.

### 7. Tests

- **Backend**: Update `createworkspace_test.go` to assert the new field is
  set correctly after workspace creation.
- **Frontend**: Update `agentWorkspaceUtils.test.ts` to use the new boolean
  instead of timestamp comparisons.
- Remove `isWorkspaceAutoCreated` function and its tests (no longer needed).

### 8. Verify

- `make gen` succeeds.
- `make lint` passes.
- Frontend tests pass (`pnpm test` in site/).
