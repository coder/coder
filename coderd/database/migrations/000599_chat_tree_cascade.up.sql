-- Deleting a chat removes its whole subtree: named children and subagents
-- follow their parent instead of becoming parentless rows.
ALTER TABLE chats
    DROP CONSTRAINT chats_parent_chat_id_fkey,
    DROP CONSTRAINT chats_root_chat_id_fkey;

ALTER TABLE chats
    ADD CONSTRAINT chats_parent_chat_id_fkey
        FOREIGN KEY (parent_chat_id) REFERENCES chats(id) ON DELETE CASCADE,
    ADD CONSTRAINT chats_root_chat_id_fkey
        FOREIGN KEY (root_chat_id) REFERENCES chats(id) ON DELETE CASCADE;

-- Parentless user chats are the rows the tree root adopts lazily.
CREATE INDEX idx_chats_tree_adoptable
    ON chats (owner_id, organization_id)
    WHERE kind = 'chat' AND parent_chat_id IS NULL;

-- chat_subtree returns a chat and every descendant reached through
-- parent_chat_id, bounded by the tree depth limit plus one subagent level.
-- It exists so per-row LATERAL joins can walk a subtree with a recursive
-- query, which the query generator cannot express inline.
CREATE FUNCTION chat_subtree(top_chat_id uuid) RETURNS TABLE(id uuid, status chat_status, depth integer)
    LANGUAGE plpgsql STABLE
    AS $$
BEGIN
    RETURN QUERY
    WITH RECURSIVE subtree AS (
        SELECT c.id, c.status, 0 AS depth
        FROM chats c
        WHERE c.id = top_chat_id
        UNION ALL
        SELECT c.id, c.status, subtree.depth + 1
        FROM chats c
        JOIN subtree ON c.parent_chat_id = subtree.id
        WHERE subtree.depth < 6
    )
    SELECT subtree.id, subtree.status, subtree.depth FROM subtree;
END;
$$;
