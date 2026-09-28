-- A file belongs to one chat. Drop any duplicate links, keeping the link on
-- the oldest chat, before enforcing it.
DELETE FROM chat_file_links l
USING chat_file_links o, chats lc, chats oc
WHERE o.file_id = l.file_id
  AND lc.id = l.chat_id
  AND oc.id = o.chat_id
  AND (oc.created_at, oc.id) < (lc.created_at, lc.id);

ALTER TABLE chat_file_links
    ADD CONSTRAINT chat_file_links_file_id_key UNIQUE (file_id);

-- The unique index replaces the purge lookup index.
DROP INDEX idx_chat_file_links_file_id;
