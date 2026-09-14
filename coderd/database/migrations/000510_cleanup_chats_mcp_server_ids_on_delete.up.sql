-- Remove already-stale MCP server references before future deletes are
-- handled by the trigger below.
UPDATE chats
SET mcp_server_ids = (
	SELECT COALESCE(array_agg(ids.mcp_server_id ORDER BY ids.position), '{}'::uuid[])
	FROM unnest(chats.mcp_server_ids) WITH ORDINALITY AS ids(mcp_server_id, position)
	WHERE EXISTS (
		SELECT 1
		FROM mcp_server_configs
		WHERE mcp_server_configs.id = ids.mcp_server_id
	)
)
WHERE EXISTS (
	SELECT 1
	FROM unnest(chats.mcp_server_ids) AS ids(mcp_server_id)
	WHERE NOT EXISTS (
		SELECT 1
		FROM mcp_server_configs
		WHERE mcp_server_configs.id = ids.mcp_server_id
	)
);

CREATE OR REPLACE FUNCTION remove_mcp_server_config_id_from_chats()
	RETURNS TRIGGER AS
$$
BEGIN
	UPDATE chats
	SET mcp_server_ids = array_remove(mcp_server_ids, OLD.id)
	WHERE OLD.id = ANY(mcp_server_ids);
	RETURN OLD;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER remove_chat_mcp_server_config_id
	BEFORE DELETE ON mcp_server_configs FOR EACH ROW
	EXECUTE PROCEDURE remove_mcp_server_config_id_from_chats();

COMMENT ON TRIGGER
	remove_chat_mcp_server_config_id
	ON mcp_server_configs IS
		'When an MCP server config is deleted, this trigger removes its ID from all chats.';
