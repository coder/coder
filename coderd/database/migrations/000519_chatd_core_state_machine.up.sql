-- Adds the core chat state-machine storage model.
-- Adds new versioning fields to chats, a revision column to chat_messages,
-- positional ordering and creator tracking to chat_queued_messages, an
-- unlogged chat_heartbeats table for ownership leases, and Postgres
-- triggers that keep history/queue versioning consistent.

-- 1. Add `interrupting` to the chat_status enum.
ALTER TYPE chat_status ADD VALUE IF NOT EXISTS 'interrupting';

-- 2. Add new versioning, ownership, retry, and pending-action fields to chats.
ALTER TABLE chats
    ADD COLUMN snapshot_version bigint NOT NULL DEFAULT 1,
    ADD COLUMN history_version bigint NOT NULL DEFAULT 0,
    ADD COLUMN queue_version bigint NOT NULL DEFAULT 0,
    ADD COLUMN generation_attempt bigint NOT NULL DEFAULT 0,
    ADD COLUMN retry_state jsonb,
    ADD COLUMN retry_state_version bigint NOT NULL DEFAULT 0,
    ADD COLUMN runner_id uuid,
    ADD COLUMN requires_action_deadline_at timestamp with time zone;

COMMENT ON COLUMN chats.snapshot_version IS
    'Monotonic version for the full chat snapshot. Starts at 1 so stream loops and workers can use 0 to mean they have not loaded the chat yet.';
COMMENT ON COLUMN chats.history_version IS
    'Snapshot version of the latest durable history change. Starts at 0 until chat_messages triggers set it to the current snapshot_version.';
COMMENT ON COLUMN chats.queue_version IS
    'Snapshot version of the latest queued-message change. Starts at 0 until chat_queued_messages triggers set it to the current snapshot_version.';

-- 3. Add `revision` to chat_messages. Adding the column as NOT NULL with
-- a constant default backfills existing rows through catalog metadata
-- only, so the highest-volume table is neither rewritten nor scanned for
-- NOT NULL validation while under ACCESS EXCLUSIVE. The default is
-- dropped immediately because the BEFORE INSERT trigger below rejects
-- inserts that pre-assign revision and assigns it from
-- chats.snapshot_version instead.
ALTER TABLE chat_messages
    ADD COLUMN revision bigint NOT NULL DEFAULT 1;
ALTER TABLE chat_messages
    ALTER COLUMN revision DROP DEFAULT;

-- 4. Backfill chats.history_version = 1 for chats that already have at
-- least one message. We avoid recursive trigger fire by performing the
-- backfill before the triggers are created.
UPDATE chats
SET history_version = 1
WHERE EXISTS (
    SELECT 1 FROM chat_messages WHERE chat_messages.chat_id = chats.id
);

-- 5. Add `position` and `created_by` to chat_queued_messages.
ALTER TABLE chat_queued_messages
    ADD COLUMN position bigint,
    ADD COLUMN created_by uuid;

-- 6. Backfill chat_queued_messages.position per chat using row_number(),
-- ordering by created_at and breaking ties by id.
WITH ordered AS (
    SELECT
        id,
        row_number() OVER (
            PARTITION BY chat_id
            ORDER BY created_at, id
        ) AS rn
    FROM chat_queued_messages
)
UPDATE chat_queued_messages
SET position = ordered.rn
FROM ordered
WHERE chat_queued_messages.id = ordered.id;

-- 7. Backfill chat_queued_messages.created_by from chats.owner_id.
UPDATE chat_queued_messages
SET created_by = chats.owner_id
FROM chats
WHERE chat_queued_messages.chat_id = chats.id
  AND chat_queued_messages.created_by IS NULL;

-- 8. Enforce NOT NULL on chat_queued_messages.position and
-- created_by. Legacy queued-message inserts are updated to populate
-- created_by from the chat owner when no explicit creator exists.
ALTER TABLE chat_queued_messages
    ALTER COLUMN position SET NOT NULL,
    ALTER COLUMN created_by SET NOT NULL;

-- 9. Default sequence for new queued-message positions.
-- A global sequence is acceptable because ordering only needs to be
-- stable within a chat.
CREATE SEQUENCE IF NOT EXISTS chat_queued_messages_position_seq AS bigint START WITH 1;
SELECT setval(
    'chat_queued_messages_position_seq',
    GREATEST((SELECT COALESCE(MAX(position), 0) FROM chat_queued_messages), 1)
);
ALTER TABLE chat_queued_messages
    ALTER COLUMN position SET DEFAULT nextval('chat_queued_messages_position_seq');

-- 10. Backfill chats.queue_version = 1 for chats that already have queued
-- messages. Same trigger-avoidance reasoning as for history_version.
UPDATE chats
SET queue_version = 1
WHERE EXISTS (
    SELECT 1 FROM chat_queued_messages WHERE chat_queued_messages.chat_id = chats.id
);

-- 11. chat_heartbeats: unlogged table for ownership leases. Keyed by
-- (chat_id, runner_id) so a single chat can briefly have entries from
-- multiple runners during failover.
CREATE UNLOGGED TABLE IF NOT EXISTS chat_heartbeats (
    chat_id uuid NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    runner_id uuid NOT NULL,
    heartbeat_at timestamp with time zone NOT NULL,
    PRIMARY KEY (chat_id, runner_id)
);

COMMENT ON TABLE chat_heartbeats IS
    'Ephemeral runner ownership leases for runnable chats. The table is unlogged because losing heartbeat rows after a crash is safe: missing heartbeats are treated as stale ownership and cause workers to reacquire runnable chats.';

CREATE INDEX IF NOT EXISTS chat_heartbeats_heartbeat_at_idx
    ON chat_heartbeats (heartbeat_at);

-- 12. Message revision trigger.
-- The BEFORE-trigger only assigns NEW.revision from chats.snapshot_version
-- and validates immutability. The chats.history_version /
-- generation_attempt update is performed by an AFTER STATEMENT trigger
-- so it doesn't conflict with CTE updates on the chats row in the same
-- command (the legacy InsertChatMessages query updates last_model_config_id
-- in a CTE on chats and then inserts messages).
CREATE FUNCTION set_chat_message_revision_before()
RETURNS trigger AS $$
DECLARE
    chat_snapshot_version bigint;
BEGIN
    IF TG_OP = 'INSERT' AND NEW.revision IS NOT NULL THEN
        RAISE EXCEPTION 'chat_messages.revision must be assigned by trigger';
    END IF;

    IF TG_OP = 'UPDATE' THEN
        IF OLD.chat_id IS DISTINCT FROM NEW.chat_id THEN
            RAISE EXCEPTION 'chat_messages.chat_id is immutable';
        END IF;

        IF OLD.revision IS DISTINCT FROM NEW.revision THEN
            RAISE EXCEPTION 'chat_messages.revision must be assigned by trigger';
        END IF;

        IF OLD IS NOT DISTINCT FROM NEW THEN
            RETURN NEW;
        END IF;
    END IF;

    SELECT snapshot_version INTO chat_snapshot_version
    FROM chats WHERE id = NEW.chat_id;

    IF chat_snapshot_version IS NULL THEN
        RAISE EXCEPTION 'chat % does not exist', NEW.chat_id;
    END IF;

    NEW.revision = chat_snapshot_version;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- AFTER STATEMENT trigger functions. Use the transition tables to
-- update chats.history_version / generation_attempt once per chat per
-- command. Running AFTER row inserts/updates complete lets a CTE
-- update on the same chats row in the same command finalize before
-- this trigger needs to update it.
--
-- The INSERT and UPDATE variants are split so the UPDATE variant can
-- reference both the OLD and NEW transition tables and skip rows that
-- did not actually change. Without that filter, a no-op UPDATE on a
-- chat_messages row (one whose OLD IS NOT DISTINCT FROM NEW) would
-- still advance chats.history_version whenever the chat's snapshot
-- had previously been bumped.
CREATE FUNCTION update_chat_history_after_message_insert()
RETURNS trigger AS $$
BEGIN
    UPDATE chats c
    SET history_version = c.snapshot_version,
        generation_attempt = 0
    FROM (
        SELECT DISTINCT chat_id FROM chat_message_history_new_rows
    ) AS affected
    WHERE c.id = affected.chat_id
      AND (
          c.history_version IS DISTINCT FROM c.snapshot_version
          OR c.generation_attempt <> 0
      );
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE FUNCTION update_chat_history_after_message_update()
RETURNS trigger AS $$
BEGIN
    UPDATE chats c
    SET history_version = c.snapshot_version,
        generation_attempt = 0
    FROM (
        SELECT DISTINCT n.chat_id
        FROM chat_message_history_new_rows n
        JOIN chat_message_history_old_rows o ON o.id = n.id
        WHERE o IS DISTINCT FROM n
    ) AS affected
    WHERE c.id = affected.chat_id
      AND (
          c.history_version IS DISTINCT FROM c.snapshot_version
          OR c.generation_attempt <> 0
      );
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_set_chat_message_revision_on_insert
BEFORE INSERT ON chat_messages
FOR EACH ROW
EXECUTE FUNCTION set_chat_message_revision_before();

CREATE TRIGGER trigger_set_chat_message_revision_on_update
BEFORE UPDATE ON chat_messages
FOR EACH ROW
EXECUTE FUNCTION set_chat_message_revision_before();

CREATE TRIGGER trigger_update_chat_history_after_message_insert
AFTER INSERT ON chat_messages
REFERENCING NEW TABLE AS chat_message_history_new_rows
FOR EACH STATEMENT
EXECUTE FUNCTION update_chat_history_after_message_insert();

CREATE TRIGGER trigger_update_chat_history_after_message_update
AFTER UPDATE ON chat_messages
REFERENCING OLD TABLE AS chat_message_history_old_rows NEW TABLE AS chat_message_history_new_rows
FOR EACH STATEMENT
EXECUTE FUNCTION update_chat_history_after_message_update();

-- 13. Queue version trigger function.
CREATE FUNCTION bump_chat_queue_version_on_queued_message_change()
RETURNS trigger AS $$
DECLARE
    changed_chat_id uuid;
BEGIN
    IF TG_OP = 'DELETE' THEN
        changed_chat_id = OLD.chat_id;
    ELSE
        changed_chat_id = NEW.chat_id;
    END IF;

    UPDATE chats
    SET queue_version = snapshot_version
    WHERE id = changed_chat_id;

    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_bump_chat_queue_version_on_queued_message_insert
AFTER INSERT ON chat_queued_messages
FOR EACH ROW
EXECUTE FUNCTION bump_chat_queue_version_on_queued_message_change();

CREATE TRIGGER trigger_bump_chat_queue_version_on_queued_message_update
AFTER UPDATE OF content, model_config_id, position, created_by
ON chat_queued_messages
FOR EACH ROW
EXECUTE FUNCTION bump_chat_queue_version_on_queued_message_change();

CREATE TRIGGER trigger_bump_chat_queue_version_on_queued_message_delete
AFTER DELETE ON chat_queued_messages
FOR EACH ROW
EXECUTE FUNCTION bump_chat_queue_version_on_queued_message_change();

-- 14. Retry state trigger function.
CREATE FUNCTION sync_chat_retry_state()
RETURNS trigger AS $$
BEGIN
    IF OLD.retry_state_version IS DISTINCT FROM NEW.retry_state_version THEN
        RAISE EXCEPTION 'chats.retry_state_version must be assigned by trigger';
    END IF;

    IF NEW.generation_attempt IS DISTINCT FROM OLD.generation_attempt THEN
        NEW.retry_state = NULL;
    END IF;

    IF NEW.retry_state IS DISTINCT FROM OLD.retry_state THEN
        NEW.retry_state_version = NEW.snapshot_version;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_sync_chat_retry_state
BEFORE UPDATE OF retry_state, retry_state_version, generation_attempt
ON chats
FOR EACH ROW
EXECUTE FUNCTION sync_chat_retry_state();

-- 15. Index for the chat worker acquisition scan, which runs every 30
-- seconds per replica plus on every worker wake. Leading on status lets
-- the scan touch only rows in the worker-runnable status set instead of
-- sequentially scanning the ever-growing chats table. The status set is
-- intentionally not part of the index predicate: 'interrupting' is added
-- to chat_status above, and Postgres forbids using a new enum value in
-- the same transaction, which all migrations share.
CREATE INDEX idx_chats_worker_acquisition_candidates ON chats
    USING btree (status, updated_at, id)
    WHERE archived = false;

-- 16. Refresh chats_expanded to include the new chat fields. Drop and
-- recreate so column ordering is stable.
DROP VIEW IF EXISTS chats_expanded;
CREATE VIEW chats_expanded AS
SELECT
    c.id,
    c.owner_id,
    c.workspace_id,
    c.title,
    c.status,
    c.worker_id,
    c.started_at,
    c.heartbeat_at,
    c.created_at,
    c.updated_at,
    c.parent_chat_id,
    c.root_chat_id,
    c.last_model_config_id,
    c.archived,
    c.last_error,
    c.mode,
    c.mcp_server_ids,
    c.labels,
    c.build_id,
    c.agent_id,
    c.pin_order,
    c.last_read_message_id,
    c.last_injected_context,
    c.dynamic_tools,
    c.organization_id,
    c.plan_mode,
    c.client_type,
    c.last_turn_summary,
    c.snapshot_version,
    c.history_version,
    c.queue_version,
    c.generation_attempt,
    c.retry_state,
    c.retry_state_version,
    c.runner_id,
    c.requires_action_deadline_at,
    COALESCE(root.user_acl, c.user_acl) AS user_acl,
    COALESCE(root.group_acl, c.group_acl) AS group_acl,
    owner.username AS owner_username,
    owner.name AS owner_name
FROM
    chats c
    LEFT JOIN chats root ON root.id = COALESCE(c.root_chat_id, c.parent_chat_id)
    JOIN visible_users owner ON owner.id = c.owner_id;
