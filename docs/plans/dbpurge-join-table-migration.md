# Plan: Update PR 23833 (dbpurge) for chat_file_links Join Table

**Research:** [dbpurge-join-table-migration-research.md](./dbpurge-join-table-migration-research.md)

## Problem

PR 23833 (`cian/dbpurge-chat-files`) adds periodic purging of chats and chat_files. It depends on PR 23537 (`cian/files-tab-rhs-panel`), which was refactored from a `chats.file_ids` UUID array column to a normalized `chat_file_links` join table with FK cascades. PR 23833 must be updated to match the new schema.

## Decisions

| # | Question                                              | Chosen                                 | Classification    | Reasoning                                                                                                                                                                                                            |
|---|-------------------------------------------------------|----------------------------------------|-------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| 1 | How to find referenced files in `DeleteOldChatFiles`? | JOIN `chat_file_links` with `chats`    | Agent-recommended | Direct semantic mapping from old `unnest(file_ids)` approach. The `idx_chat_file_links_chat_id` index ensures performance. See research: "DeleteOldChatFiles — rewrite to JOIN chat_file_links".                     |
| 2 | Keep or remove scrubbing in `UnarchiveChatByID`?      | Remove scrubbing, simplify query       | Agent-recommended | FK cascade on `chat_file_links_file_id_fkey` guarantees cleanup when files are deleted. Application-level scrubbing is redundant. See research: "UnarchiveChatByID — simplify".                                      |
| 3 | What to do with `UnarchiveScrubsStaleFileIDs` test?   | Repurpose as `UnarchiveAfterFilePurge` | Agent-recommended | Validates the same user-facing invariant (unarchived chats don't reference purged files) via FK cascades instead of application scrubbing. See research: "UnarchiveScrubsStaleFileIDs test — repurpose or simplify". |

## Implementation Flow

### Phase 1: Rebase and resolve conflicts

Rebase `cian/dbpurge-chat-files` onto the updated `cian/files-tab-rhs-panel`. Resolve merge conflicts mechanically where possible. The SQL files and generated code will have the most conflicts.

**Verify:** Branch compiles. Migration numbering is correct (23833's migration must come after 000459).

### Phase 2: Rewrite SQL queries

Two queries need rewriting:

1. **`DeleteOldChatFiles`** — Replace the `kept_file_ids` CTE: instead of `unnest(file_ids)` from `chats`, JOIN `chat_file_links` with `chats`. Remove the `file_ids <> '{}'` guard (the JOIN handles this naturally).

2. **`UnarchiveChatByID`** — Remove the `file_ids = COALESCE(...)` scrubbing subquery. The query becomes a simple `UPDATE ... SET archived = false, updated_at = NOW()`.

After rewriting, regenerate sqlc output (`make gen`).

**Verify:** `make gen` succeeds. The generated Go types for `DeleteOldChatFilesParams` remain `{BeforeTime, LimitCount}`. `UnarchiveChatByID` return type no longer needs to carry file_ids.

### Phase 3: Update Go code

1. **`dbpurge.go`** — No changes expected. The Go code calls `tx.DeleteOldChats()` and `tx.DeleteOldChatFiles()` with the same parameter shapes.

2. **`dbauthz.go`** — Verify that auth wrappers for `UnarchiveChatByID` still compile. The simplified query shouldn't change the auth contract.

3. **`exp_chats.go`** — Verify the retention-days handler and unarchive handler still compile. The `Chat` struct no longer has a `FileIDs` field (confirmed — see research: "What Changed in 23537"), so any code that accessed it will fail to compile and must be removed.

**Verify:** `go build ./...` succeeds.

### Phase 4: Update tests

1. **Replace all `db.AppendChatFileIDs()` calls** with `db.LinkChatFiles()` in `dbpurge_test.go`. Update parameter struct from `AppendChatFileIDsParams` to `LinkChatFilesParams`. Note: `LinkChatFiles` returns `(rejected_new_files int32, error)` rather than `(int64, error)`. In tests the cap is set to 100 (well above actual file counts), so no assertion changes are needed for the return value — just discard the `rejected` return.

2. **Replace all `updated.FileIDs` assertions** with queries via `GetChatFileMetadataByChatID` to check which files are still linked to a chat. The assertion pattern changes from checking an array field on the Chat struct to querying the join table for linked file metadata.

3. **Repurpose `UnarchiveScrubsStaleFileIDs`** → `UnarchiveAfterFilePurge`:
   - Keep the setup (create chat with files, archive, delete files)
   - Remove assertions about `FileIDs` field scrubbing
   - Instead verify that `GetChatFileMetadataByChatID` returns only surviving files
   - This tests the end-to-end flow: file deletion → FK cascade cleans links → unarchived chat sees correct files

4. **`OrphanedOldFilesDeleted` and `ArchivedChatFilesDeleted`** — Only the file-linking calls change. The assertion pattern (check if file exists via `GetChatFileByID`) remains the same.

**Verify:** `go test ./coderd/database/dbpurge/ -count=1` passes. All sub-tests of `TestDeleteOldChatFiles` pass: `ChatRetentionDisabled`, `OldArchivedChatsDeleted`, `OrphanedOldFilesDeleted`, `ArchivedChatFilesDeleted`, `UnarchiveAfterFilePurge` (renamed), `BatchLimitFiles`, `BatchLimitChats`.

### Phase 5: Regenerate and verify

1. Run `make gen` to regenerate all codegen (sqlc, dbmock, dbmetrics, dbauthz).
2. Run `make lint` to catch any issues.
3. Run the full test suite for affected packages.

**Verify:** CI-equivalent checks pass locally.
