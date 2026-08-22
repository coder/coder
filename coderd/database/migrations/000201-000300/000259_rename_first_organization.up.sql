UPDATE
	organizations
SET
	name = 'coder',
	display_name = 'Coder'
WHERE
	-- The old name was too long.
	name = 'first-organization'
	AND is_default = true
;
