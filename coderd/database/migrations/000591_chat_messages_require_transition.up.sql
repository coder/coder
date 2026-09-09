-- Every chat_messages insert or update must happen inside a chat state
-- transition. A transition writes the chats row first, under its row lock,
-- to allocate a new snapshot_version; the history triggers below then set
-- history_version to that snapshot, and the transition publishes a state
-- update after commit. A write from outside a transition moves
-- history_version alone: no snapshot bump, no state update, so the chat
-- runner and the chat stream see a row whose version has not changed and
-- ignore it. One such writer left a running chat with no task and a live
-- heartbeat until a user acted.
--
-- A row written in the current transaction carries the transaction's id in
-- xmin. The triggers now refuse a counted chat_messages write for any chat
-- whose row this transaction has not written. Updates that only touch the
-- search index columns are not counted, as before.

CREATE OR REPLACE FUNCTION update_chat_history_after_message_insert() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    offending_chat_id uuid;
BEGIN
    SELECT c.id INTO offending_chat_id
    FROM chats c
    WHERE c.id IN (SELECT DISTINCT chat_id FROM chat_message_history_new_rows)
      AND c.xmin <> pg_current_xact_id()::xid
    LIMIT 1;
    IF offending_chat_id IS NOT NULL THEN
        RAISE EXCEPTION 'chat_messages for chat % written outside a chat state transition', offending_chat_id
            USING HINT = 'Write chat history through chatstate.ChatMachine.Update or chatstate.CreateChat, which write the chats row in the same transaction first.';
    END IF;

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

COMMENT ON FUNCTION update_chat_history_after_message_insert() IS 'Component of chatd. Updates history_version and generation_attempt on chats when chat_messages rows are inserted. Rejects inserts made outside a chat state transition, detected as a chats row not written in the current transaction.';

CREATE OR REPLACE FUNCTION update_chat_history_after_message_update() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    offending_chat_id uuid;
BEGIN
    SELECT c.id INTO offending_chat_id
    FROM chats c
    WHERE c.id IN (
        SELECT DISTINCT n.chat_id
        FROM chat_message_history_new_rows n
        JOIN chat_message_history_old_rows o ON o.id = n.id
        WHERE (to_jsonb(o) - 'search_tsv' - 'search_tsv_config') IS DISTINCT FROM (to_jsonb(n) - 'search_tsv' - 'search_tsv_config')
    )
      AND c.xmin <> pg_current_xact_id()::xid
    LIMIT 1;
    IF offending_chat_id IS NOT NULL THEN
        RAISE EXCEPTION 'chat_messages for chat % written outside a chat state transition', offending_chat_id
            USING HINT = 'Write chat history through chatstate.ChatMachine.Update or chatstate.CreateChat, which write the chats row in the same transaction first.';
    END IF;

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

COMMENT ON FUNCTION update_chat_history_after_message_update() IS 'Component of chatd. Updates history_version and generation_attempt on chats when chat_messages is updated. Excludes changes to search_tsv and search_tsv_config. Rejects updates made outside a chat state transition, detected as a chats row not written in the current transaction.';
