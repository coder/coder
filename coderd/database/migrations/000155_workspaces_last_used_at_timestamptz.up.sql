ALTER TABLE ONLY workspaces
ALTER COLUMN last_used_at
	SET DATA TYPE timestamptz
	USING last_used_at::timestamp AT TIME ZONE 'UTC',
ALTER COLUMN last_used_at
	SET DEFAULT '0001-01-01 00:00:00+00:00'::timestamptz;
