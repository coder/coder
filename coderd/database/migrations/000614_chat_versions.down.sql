DROP VIEW chats_expanded;

ALTER TABLE chats
    ADD COLUMN snapshot_version bigint DEFAULT 1 NOT NULL,
    ADD COLUMN history_version bigint DEFAULT 0 NOT NULL,
    ADD COLUMN queue_version bigint DEFAULT 0 NOT NULL,
    ADD COLUMN generation_attempt bigint DEFAULT 0 NOT NULL,
    ADD COLUMN retry_state jsonb,
    ADD COLUMN retry_state_version bigint DEFAULT 0 NOT NULL;

UPDATE chats c
SET snapshot_version = v.snapshot_version,
    history_version = v.history_version,
    queue_version = v.queue_version,
    generation_attempt = v.generation_attempt,
    retry_state = v.retry_state,
    retry_state_version = v.retry_state_version
FROM chat_versions v
WHERE v.chat_id = c.id;

COMMENT ON COLUMN chats.snapshot_version IS 'Monotonic version for the full chat snapshot. Starts at 1 so stream loops and workers can use 0 to mean they have not loaded the chat yet.';

COMMENT ON COLUMN chats.history_version IS 'Snapshot version of the latest durable history change. Starts at 0 until chat_messages triggers set it to the current snapshot_version.';

COMMENT ON COLUMN chats.queue_version IS 'Snapshot version of the latest queued-message change. Starts at 0 until chat_queued_messages triggers set it to the current snapshot_version.';

CREATE OR REPLACE FUNCTION set_chat_message_revision_before() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    chat_snapshot_version bigint;
    cmp chat_messages;
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

        cmp := NEW;
        cmp.search_tsv := OLD.search_tsv;
        cmp.search_tsv_config := OLD.search_tsv_config;
        IF OLD IS NOT DISTINCT FROM cmp THEN
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
$$;

CREATE OR REPLACE FUNCTION update_chat_history_after_message_insert() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
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
$$;

CREATE OR REPLACE FUNCTION update_chat_history_after_message_update() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    UPDATE chats c
    SET history_version = c.snapshot_version,
        generation_attempt = 0
    FROM (
        SELECT DISTINCT n.chat_id
        FROM chat_message_history_new_rows n
        JOIN chat_message_history_old_rows o ON o.id = n.id
        WHERE (to_jsonb(o) - 'search_tsv' - 'search_tsv_config') IS DISTINCT FROM (to_jsonb(n) - 'search_tsv' - 'search_tsv_config')
    ) AS affected
    WHERE c.id = affected.chat_id
      AND (
          c.history_version IS DISTINCT FROM c.snapshot_version
          OR c.generation_attempt <> 0
      );
    RETURN NULL;
END;
$$;

CREATE OR REPLACE FUNCTION bump_chat_queue_version_on_queued_message_change() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
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
$$;

DROP TRIGGER trigger_sync_chat_retry_state ON chat_versions;
CREATE TRIGGER trigger_sync_chat_retry_state BEFORE UPDATE OF retry_state, retry_state_version, generation_attempt ON chats FOR EACH ROW EXECUTE FUNCTION sync_chat_retry_state();

DROP TABLE chat_versions;

CREATE VIEW chats_expanded AS
 SELECT c.id,
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
    c.last_reasoning_effort,
    c.archived,
    c.last_error,
    c.mode,
    c.mcp_server_ids,
    c.labels,
    c.build_id,
    c.agent_id,
    c.pin_order,
    c.last_read_message_id,
    c.dynamic_tools,
    c.organization_id,
    c.project_id,
    c.plan_mode,
    c.client_type,
    c.last_turn_summary,
    c.summary,
    c.summary_generated_at,
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
    owner.name AS owner_name,
    c.context_aggregate_hash,
    c.context_dirty_since,
    c.context_dirty_resources,
    c.context_error,
    c.compaction_requested_at,
    c.title_source,
    c.title_updated_at,
    c.automation_id,
    c.manage_automations_enabled
   FROM ((chats c
     LEFT JOIN chats root ON ((root.id = COALESCE(c.root_chat_id, c.parent_chat_id))))
     JOIN visible_users owner ON ((owner.id = c.owner_id)));
