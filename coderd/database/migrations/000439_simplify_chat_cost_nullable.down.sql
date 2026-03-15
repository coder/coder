-- Restore NULL for unpriced rows.
UPDATE chat_messages SET total_cost_micros = NULL WHERE cost_valid = false;

-- Remove NOT NULL constraint and default.
ALTER TABLE chat_messages ALTER COLUMN total_cost_micros DROP NOT NULL;
ALTER TABLE chat_messages ALTER COLUMN total_cost_micros DROP DEFAULT;

-- Drop cost_valid column.
ALTER TABLE chat_messages DROP COLUMN cost_valid;
