CREATE OR REPLACE FUNCTION tags_compatible(subset_tags jsonb, superset_tags jsonb)
RETURNS boolean as $$
BEGIN
	RETURN CASE
		WHEN superset_tags = '{"scope": "organization", "owner": ""}' :: jsonb
		THEN subset_tags = superset_tags
		ELSE subset_tags <@ superset_tags
	END;
END;
$$ LANGUAGE plpgsql;
