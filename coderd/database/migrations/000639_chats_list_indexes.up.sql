-- Serves GetChats: one owner's root chats in sidebar order. archived is a
-- key column so the archived and unarchived lists share one index.
CREATE INDEX idx_chats_owner_root_list ON chats (
	owner_id,
	archived,
	(CASE WHEN pin_order > 0 THEN 1 ELSE 0 END) DESC,
	(-pin_order) DESC,
	updated_at DESC,
	id DESC
) WHERE parent_chat_id IS NULL;

-- Serves the GetChats shared-with-me filter. Only root chats carry ACL
-- entries (chat_acl_only_on_root_chats), and few of them are shared.
CREATE INDEX idx_chats_shared_roots ON chats (id)
	WHERE parent_chat_id IS NULL
		AND (user_acl <> '{}'::jsonb OR group_acl <> '{}'::jsonb);

-- The RBAC SQL filter compares owner_id::text and organization_id::text.
-- Without statistics on those expressions the planner underestimates how
-- many chats pass it by about 150x and sorts all of a large owner's chats
-- instead of reading idx_chats_owner_root_list in order. Expression
-- statistics require PostgreSQL 14, and ANALYZE fills them right away.
-- Databases that run this migration on PostgreSQL 13 do not get them after
-- a later server upgrade, and dump.sql (generated on 13) omits them.
DO $$
BEGIN
	IF current_setting('server_version_num')::int >= 140000 THEN
		CREATE STATISTICS chats_rbac_text_casts ON (owner_id::text), (organization_id::text) FROM chats;
		ANALYZE chats;
	END IF;
END
$$;
