-- Restore NULL cost for rows that new code marked as unpriced.
UPDATE chat_messages SET total_cost_micros = NULL WHERE cost_valid = false;

-- Drop cost_valid column.
ALTER TABLE chat_messages DROP COLUMN cost_valid;
