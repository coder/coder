-- Add cost_valid column. Default false = unpriced.
ALTER TABLE chat_messages ADD COLUMN cost_valid boolean NOT NULL DEFAULT false;

-- Backfill: any row with non-NULL total_cost_micros is priced.
UPDATE chat_messages SET cost_valid = true WHERE total_cost_micros IS NOT NULL;

-- Convert NULL costs to 0 so column can become NOT NULL.
UPDATE chat_messages SET total_cost_micros = 0 WHERE total_cost_micros IS NULL;

-- Make total_cost_micros non-nullable with default 0.
ALTER TABLE chat_messages ALTER COLUMN total_cost_micros SET NOT NULL;
ALTER TABLE chat_messages ALTER COLUMN total_cost_micros SET DEFAULT 0;
