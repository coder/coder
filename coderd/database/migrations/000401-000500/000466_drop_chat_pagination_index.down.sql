CREATE INDEX idx_chats_owner_updated_id ON chats (owner_id, updated_at DESC, id DESC);
