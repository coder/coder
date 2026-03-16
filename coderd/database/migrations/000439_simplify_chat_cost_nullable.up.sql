-- Add cost_valid as a nullable column with no default for
-- mixed-version rollout compatibility. Old writers that do not
-- know about this column will insert NULL, which the summary
-- query interprets via COALESCE as falling back to the
-- total_cost_micros IS NOT NULL heuristic.
ALTER TABLE chat_messages ADD COLUMN cost_valid boolean;

-- Backfill: mark existing rows based on whether they have a cost.
UPDATE chat_messages SET cost_valid = (total_cost_micros IS NOT NULL);
