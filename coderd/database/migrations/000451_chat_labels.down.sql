DROP INDEX IF EXISTS idx_chats_labels;

ALTER TABLE chats DROP COLUMN labels;
