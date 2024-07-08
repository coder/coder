-- This ensures a default everyone group exists for default org.
INSERT INTO
	groups(name, id, organization_id)
SELECT
	-- This is a special keyword that must be exactly this.
	'Everyone',
	-- Org ID and group ID must match.
	(SELECT id FROM organizations WHERE is_default = true LIMIT 1),
	(SELECT id FROM organizations WHERE is_default = true LIMIT 1)
-- It might already exist
ON CONFLICT DO NOTHING;
