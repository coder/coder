-- Restricts capacity count scans to chats that can hold a slot.
CREATE INDEX idx_chats_capacity_active ON chats (parent_chat_id)
    WHERE archived = false
      AND status IN ('running', 'interrupting')
      AND worker_id IS NOT NULL
      AND runner_id IS NOT NULL;
