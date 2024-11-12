-- We need this as a type alias to ensure that we can override it in sqlc.
-- We need to override it in sqlc so that it can be recognised as a StringMap.
-- Without this zero values for other inferred types cause json syntax errors.
CREATE DOMAIN tags AS jsonb;

CREATE OR REPLACE FUNCTION tags_compatible(subset_tags tags, superset_tags tags)
RETURNS boolean as $$
BEGIN
	RETURN CASE
		-- Special case for untagged provisioners
		WHEN subset_tags :: jsonb = '{"scope": "organization", "owner": ""}' :: jsonb
		THEN subset_tags = superset_tags
		ELSE subset_tags :: jsonb <@ superset_tags :: jsonb
	END;
END;
$$ LANGUAGE plpgsql;
