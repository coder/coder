UPDATE
	organizations
SET
	name = 'main',
	display_name = 'Main'
WHERE
	-- The old name was too long.
	name = 'first-organization'
;
