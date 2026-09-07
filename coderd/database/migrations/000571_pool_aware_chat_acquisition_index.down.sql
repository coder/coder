DROP INDEX idx_chats_worker_acquisition_candidates;
CREATE INDEX idx_chats_worker_acquisition_candidates ON chats
    (status, updated_at, id)
    WHERE archived = false;
