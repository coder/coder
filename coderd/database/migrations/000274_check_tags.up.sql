CREATE DOMAIN tags AS jsonb;

CREATE OR REPLACE FUNCTION tags_compatible(subset_tags tags, superset_tags tags)
RETURNS boolean as $$
BEGIN
	RETURN CASE
		WHEN superset_tags :: jsonb = '{"scope": "organization", "owner": ""}' :: jsonb
		THEN subset_tags = superset_tags
		ELSE subset_tags :: jsonb <@ superset_tags :: jsonb
	END;
END;
$$ LANGUAGE plpgsql;
