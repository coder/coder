-- The GetChats ORDER BY changed from (updated_at, id) DESC to a 4-column
-- expression sort (pinned-first flag, negated pin_order, updated_at, id).
-- This index was purpose-built for the old sort and no longer provides
-- read benefit. The simpler idx_chats_owner covers the owner_id filter.
DROP INDEX IF EXISTS idx_chats_owner_updated_id;
