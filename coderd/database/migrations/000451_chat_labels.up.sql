ALTER TABLE chats ADD COLUMN labels jsonb NOT NULL DEFAULT '{}';

CREATE INDEX idx_chats_labels ON chats USING GIN (labels);
