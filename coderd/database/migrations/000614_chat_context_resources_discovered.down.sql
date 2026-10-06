-- Rows chatd pinned on its own are not snapshot copies; without the marker
-- the previous release would treat them as pinned context and count them as
-- drift.
DELETE FROM chat_context_resources
WHERE discovered = true;

ALTER TABLE chat_context_resources
    DROP COLUMN discovered;
