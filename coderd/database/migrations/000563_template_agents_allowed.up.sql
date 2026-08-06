ALTER TABLE templates ADD COLUMN agents_allowed boolean DEFAULT true NOT NULL;

COMMENT ON COLUMN templates.agents_allowed IS 'Whether Coder Agents can create workspaces using this template.';

DO $$
DECLARE
	raw text;
	parsed jsonb;
	parsed_ids uuid[];
BEGIN
	SELECT value INTO raw
	FROM site_configs
	WHERE key = 'agents_template_allowlist';

	IF raw IS NULL OR btrim(raw) = '' THEN
		RETURN;
	END IF;

	BEGIN
		parsed := raw::jsonb;
		IF parsed = 'null'::jsonb THEN
			RETURN;
		END IF;
		IF jsonb_typeof(parsed) <> 'array' THEN
			RAISE EXCEPTION 'value is not a JSON array';
		END IF;
		IF jsonb_array_length(parsed) = 0 THEN
			RETURN;
		END IF;

		SELECT array_agg(entry::uuid)
		INTO parsed_ids
		FROM jsonb_array_elements_text(parsed) AS entries(entry);

		IF array_position(parsed_ids, NULL) IS NOT NULL THEN
			RAISE EXCEPTION 'contains a null template ID';
		END IF;
	EXCEPTION WHEN others THEN
		RAISE WARNING 'agents_template_allowlist is corrupt (%); blocking all templates', SQLERRM;
		parsed_ids := ARRAY[]::uuid[];
	END;

	-- A valid nonempty list allows matching existing templates only. Missing, null,
	-- or empty data leaves templates allowed. Corrupt data blocks all templates.
	UPDATE templates
	SET agents_allowed = (id = ANY(parsed_ids));
END $$;

-- As usual, recreate the view so templates.* is expanded to include the new column.
DROP VIEW template_with_names;

CREATE VIEW template_with_names AS
SELECT templates.*,
	   COALESCE(visible_users.avatar_url, ''::text) AS created_by_avatar_url,
	   COALESCE(visible_users.username, ''::text) AS created_by_username,
	   COALESCE(visible_users.name, ''::text) AS created_by_name,
	   COALESCE(organizations.name, ''::text) AS organization_name,
	   COALESCE(organizations.display_name, ''::text) AS organization_display_name,
	   COALESCE(organizations.icon, ''::text) AS organization_icon
FROM ((templates
	LEFT JOIN visible_users ON ((templates.created_by = visible_users.id)))
	LEFT JOIN organizations ON ((templates.organization_id = organizations.id)));

COMMENT ON VIEW template_with_names IS 'Joins in the display name information such as username, avatar, and organization name.';
