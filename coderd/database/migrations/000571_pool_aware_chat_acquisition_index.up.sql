DROP INDEX idx_chats_worker_acquisition_candidates;
CREATE INDEX idx_chats_worker_acquisition_candidates ON chats
    ((parent_chat_id IS NULL), status, updated_at, id)
    WHERE archived = false;
