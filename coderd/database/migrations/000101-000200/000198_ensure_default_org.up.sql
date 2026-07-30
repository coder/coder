-- This ensures a default organization always exists.
INSERT INTO
	organizations(id, name, description, created_at, updated_at, is_default)
SELECT
	-- Avoid calling it "default" as we are reserving that word as a keyword to fetch
	-- the default org regardless of the name.
	gen_random_uuid(),
	'first-organization',
	'Builtin default organization.',
	now(),
	now(),
	true
WHERE
	-- Only insert if no organizations exist.
	NOT EXISTS (SELECT * FROM organizations);

