-- Restricts capacity count scans to chats that can hold or wait for a slot.
CREATE INDEX idx_chats_capacity_active ON chats (parent_chat_id)
    WHERE archived = false
      AND status IN ('running', 'interrupting');
