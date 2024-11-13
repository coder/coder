CREATE DOMAIN tagset AS jsonb;

COMMENT ON DOMAIN tagset IS 'A set of tags that match provisioner daemons to provisioner jobs, which can originate from workspaces or templates. tagset is a narrowed type over jsonb. It is expected to be the JSON representation of map[string]string. That is, {"key1": "value1", "key2": "value2"}. We need the narrowed type instead of just using jsonb so that we can give sqlc a type hint, otherwise it defaults to json.RawMessage. json.RawMessage is a suboptimal type to use in the context that we need tagset for.';

CREATE OR REPLACE FUNCTION provisioner_tagset_contains(superset tagset, subset tagset)
RETURNS boolean as $$
BEGIN
	RETURN
		-- Special case for untagged provisioners, where only an exact match should count
		(subset = '{"scope": "organization", "owner": ""}' :: tagset AND subset = superset)
		-- General case
		OR subset <@ superset;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION provisioner_tagset_contains(tagset, tagset) IS 'Returns true if the superset contains the subset, or if the subset represents an untagged provisioner and the superset is exactly equal to the subset.';
