CREATE DOMAIN tagset AS jsonb;

COMMENT ON DOMAIN tagset IS 'A set of tags that match provisioner daemons to provisioner jobs, which can originate from workspaces or templates. tagset is a narrowed type over jsonb. It is expected to be the JSON representation of map[string]string. That is, {"key1": "value1", "key2": "value2"}. We need the narrowed type instead of just using jsonb so that we can give sqlc a type hint, otherwise it defaults to json.RawMessage. json.RawMessage is a suboptimal type to use in the context that we need tagset for.';

CREATE OR REPLACE FUNCTION provisioner_tagset_contains(provisioner_tags tagset, job_tags tagset)
RETURNS boolean AS $$
BEGIN
	RETURN CASE
		-- Special case for untagged provisioners, where only an exact match should count
		WHEN job_tags::jsonb = '{"scope": "organization", "owner": ""}'::jsonb THEN job_tags::jsonb = provisioner_tags::jsonb
		-- General case
		ELSE job_tags::jsonb <@ provisioner_tags::jsonb
	END;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION provisioner_tagset_contains(tagset, tagset) IS 'Returns true if the provisioner_tags contains the job_tags, or if the job_tags represents an untagged provisioner and the superset is exactly equal to the subset.';
