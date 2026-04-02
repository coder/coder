# Research: Updating PR 23833 (dbpurge) for chat_file_links Join Table

## Problem Context

PR 23537 (`cian/files-tab-rhs-panel`) originally tracked chat↔file associations via a `file_ids uuid[]` column on the `chats` table. After review feedback ("join on `chat_files` table instead of adding `file_ids`"), this was refactored to a normalized `chat_file_links` join table. The migration (000459) explicitly **drops** the `file_ids` column.

PR 23833 (`cian/dbpurge-chat-files`) adds periodic purging of chats and chat_files to the dbpurge background goroutine. It was written against the **old** version of 23537 that used `file_ids`. Every SQL query and test that references `chats.file_ids` or `AppendChatFileIDs` is now broken against the updated base branch.

### What Changed in 23537

**Migration 000459** creates:

```sql
CREATE TABLE chat_file_links (
    chat_id uuid NOT NULL,
    file_id uuid NOT NULL,
    UNIQUE (chat_id, file_id)
);
CREATE INDEX idx_chat_file_links_chat_id ON chat_file_links (chat_id);
ALTER TABLE chat_file_links
    ADD CONSTRAINT chat_file_links_chat_id_fkey
    FOREIGN KEY (chat_id) REFERENCES chats(id) ON DELETE CASCADE;
ALTER TABLE chat_file_links
    ADD CONSTRAINT chat_file_links_file_id_fkey
    FOREIGN KEY (file_id) REFERENCES chat_files(id) ON DELETE CASCADE;
ALTER TABLE chats DROP COLUMN IF EXISTS file_ids;
```

**Key FK cascade behavior:**

- Deleting a **chat** → cascades to delete its `chat_file_links` rows (but NOT the `chat_files` themselves)
- Deleting a **chat_file** → cascades to delete its `chat_file_links` rows

**New queries replacing old ones:**

| Old (file_ids-based)                          | New (join-table-based)                        |
|-----------------------------------------------|-----------------------------------------------|
| `AppendChatFileIDs`                           | `LinkChatFiles`                               |
| `GetChatFileMetadataByIDs` (takes UUID array) | `GetChatFileMetadataByChatID` (takes chat ID) |

**`LinkChatFiles`** is a CTE-based query that atomically inserts into `chat_file_links` with deduplication and cap enforcement. Returns `rejected_new_files` count.

**`Chat` Go struct** no longer has a `FileIDs` field. The `Chat` SDK response struct now has `Files []ChatFileMetadata` populated via `GetChatFileMetadataByChatID`.

## Affected Locations in PR 23833

### SQL Queries (Verified — read from `cian/dbpurge-chat-files` branch)

**1. `DeleteOldChatFiles`** (`coderd/database/queries/chatfiles.sql`)

```sql
-- kept_file_ids CTE uses unnest(file_ids) from chats table:
SELECT DISTINCT unnest(file_ids) AS file_id
FROM chats
WHERE (archived = false
       OR updated_at >= @before_time::timestamptz)
  AND file_ids <> '{}'
```

Status: **BROKEN** — `file_ids` column no longer exists.

**2. `UnarchiveChatByID`** (`coderd/database/queries/chats.sql`)

```sql
-- Scrubs stale file_ids on unarchive:
file_ids = COALESCE(
    (SELECT array_agg(fid)
     FROM unnest(file_ids) AS fid
     WHERE EXISTS (SELECT 1 FROM chat_files WHERE id = fid)),
    '{}')
```

Status: **BROKEN** — `file_ids` column no longer exists. Additionally, the FK cascade on `chat_file_links_file_id_fkey` means stale references are automatically cleaned up when files are deleted — this scrubbing logic is no longer needed.

### Go Code (Verified — read from `cian/dbpurge-chat-files` branch)

**3. `dbpurge_test.go`** — Uses `db.AppendChatFileIDs()` in 6 places across tests:

- `OrphanedOldFilesDeleted`: links fileB to activeChat
- `ArchivedChatFilesDeleted`: links fileD, fileE, fileF to various chats
- `UnarchiveScrubsStaleFileIDs`: links fileA/B/C, parent/child files

Status: **BROKEN** — `AppendChatFileIDs` no longer exists. Must use `LinkChatFiles`.

**4. `dbpurge_test.go`** — Checks `updated.FileIDs` field:

- `UnarchiveScrubsStaleFileIDs`: asserts `updated.FileIDs == []uuid.UUID{fileC}`, asserts `updated.FileIDs` is not nil/empty, asserts parent/child `FileIDs`.

Status: **BROKEN** — `Chat` struct no longer has `FileIDs` field.

### Querier Interface (Verified)

**5. `coderd/database/querier.go`** — `AppendChatFileIDs` signature exists in 23833's branch.

Status: **BROKEN** — replaced by `LinkChatFiles` in 23537.

## Approach: Adapt to Join Table with FK Cascades

There is only one viable approach here: adapt all the queries and tests to use the join table. The schema decision has already been made and approved in 23537.

### DeleteOldChatFiles — rewrite to JOIN chat_file_links

The `kept_file_ids` CTE should query `chat_file_links` joined with `chats` instead of unnesting `file_ids`:

```sql
WITH kept_file_ids AS (
    SELECT DISTINCT cfl.file_id
    FROM chat_file_links cfl
    JOIN chats c ON c.id = cfl.chat_id
    WHERE c.archived = false
       OR c.updated_at >= @before_time::timestamptz
)
```

This is semantically identical: it finds all file IDs referenced by active or recently-archived chats. The `file_ids <> '{}'` guard is no longer needed since the JOIN naturally excludes chats with no linked files.

**Strongest argument for:** Direct 1:1 semantic mapping from the old query. The query plan should be efficient with the existing `idx_chat_file_links_chat_id` index.

**Strongest argument against:** None — this is the only correct approach given the schema.

### UnarchiveChatByID — simplify (FK cascades handle cleanup)

With the join table's FK cascade (`chat_file_links_file_id_fkey ... ON DELETE CASCADE`), when `DeleteOldChatFiles` deletes a file row from `chat_files`, the corresponding `chat_file_links` rows are **automatically** deleted by PostgreSQL.

This means the `UnarchiveChatByID` scrubbing logic is entirely unnecessary. The query simplifies to:

```sql
UPDATE chats SET
    archived = false,
    updated_at = NOW()
WHERE id = @id::uuid OR root_chat_id = @id::uuid
RETURNING *
```

**Strongest argument for:** Less code, no SQL subquery, correctness guaranteed by FK constraints rather than application-level scrubbing.

**Strongest argument against:** The scrubbing was a defense-in-depth measure. Without it, orphaned links (if they somehow survive despite FKs) would persist. However, FK constraints in PostgreSQL are transactional and reliable — this risk is negligible.

### Tests — replace AppendChatFileIDs with LinkChatFiles

All test helper calls change from:

```go
db.AppendChatFileIDs(ctx, database.AppendChatFileIDsParams{
    ChatID:     chat.ID,
    MaxFileIDs: 100,
    FileIDs:    []uuid.UUID{fileB},
})
```

to:

```go
db.LinkChatFiles(ctx, database.LinkChatFilesParams{
    ChatID:       chat.ID,
    MaxFileLinks: 100,
    FileIds:      []uuid.UUID{fileB},
})
```

### UnarchiveScrubsStaleFileIDs test — repurpose or simplify

The test verified that `UnarchiveChatByID` scrubs stale `file_ids`. With FK cascades, there's nothing to scrub. Two options:

**Option A: Remove the test entirely.** The behavior it tested (scrubbing) no longer exists. FK cascade behavior is a database invariant, not application logic.

**Option B: Repurpose as `UnarchiveAfterFilePurge`.** Verify that after files are deleted (simulating dbpurge), the chat_file_links are automatically cleaned up by FK cascade, and `GetChatFileMetadataByChatID` returns only surviving files. This validates the end-to-end flow without testing scrubbing logic.

Option B is preferable — it validates the same user-facing invariant (unarchived chats don't reference purged files) even though the mechanism changed.

## Open Questions

1. **Does `DeleteOldChats` need changes?** No — verified. It deletes from `chats` table. The FK cascade on `chat_file_links_chat_id_fkey` automatically removes the links. The chat_files themselves become orphaned and are caught by `DeleteOldChatFiles` in the same tick. Same flow as before.

2. **Does `ArchiveChatByID` need changes?** No — verified. It doesn't touch files or file references.

3. **Does the dbpurge Go code need changes?** No — the Go code in `dbpurge.go` only calls `tx.DeleteOldChats()` and `tx.DeleteOldChatFiles()`. The parameter types (`DeleteOldChatsParams`, `DeleteOldChatFilesParams`) stay the same (BeforeTime, LimitCount). Only the SQL implementation changes.

4. **Does the retention config need changes?** No — `GetChatRetentionDays` and `UpsertChatRetentionDays` are independent of the file_ids schema.

5. **Does the API handler for retention-days need changes?** No — verified. It reads/writes `site_configs` and doesn't reference files.

6. **Does the frontend need changes?** No — the `AgentSettingsBehaviorPageView.tsx` only deals with the retention days numeric input.

## Decisions

| # | Question                                              | Options                                                         | Chosen        | Classification    | Reasoning                                                                                                                         |
|---|-------------------------------------------------------|-----------------------------------------------------------------|---------------|-------------------|-----------------------------------------------------------------------------------------------------------------------------------|
| 1 | How to find referenced files in `DeleteOldChatFiles`? | (a) JOIN `chat_file_links` with `chats`                         | (a) JOIN      | Agent-recommended | Direct semantic mapping from old `unnest(file_ids)` approach. See "DeleteOldChatFiles — rewrite to JOIN chat_file_links" section. |
| 2 | Keep or remove scrubbing in `UnarchiveChatByID`?      | (a) Remove — FK cascade handles it; (b) Keep — defense-in-depth | (a) Remove    | Agent-recommended | FK cascade on `chat_file_links_file_id_fkey` guarantees cleanup. See "UnarchiveChatByID — simplify" section.                      |
| 3 | What to do with `UnarchiveScrubsStaleFileIDs` test?   | (a) Remove entirely; (b) Repurpose as `UnarchiveAfterFilePurge` | (b) Repurpose | Agent-recommended | Validates same user-facing invariant via FK cascades. See "UnarchiveScrubsStaleFileIDs test" section.                             |

All decisions are agent-recommended pending human ratification.
