CREATE OR REPLACE FUNCTION tags_compatible(provisioner_tags jsonb, required_tags jsonb)
RETURNS boolean as $$
BEGIN
	RETURN CASE
		WHEN provisioner_tags = '{"scope": "organization", "owner": ""}' :: jsonb
		THEN provisioner_tags = required_tags
		ELSE required_tags <@ provisioner_tags
	END;
END;
$$ LANGUAGE plpgsql;
