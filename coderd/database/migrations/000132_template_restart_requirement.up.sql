BEGIN;

ALTER TABLE templates
	DROP COLUMN max_ttl,
	ADD COLUMN restart_requirement_days_of_week smallint NOT NULL DEFAULT 0,
	ADD COLUMN restart_requirement_weeks bigint NOT NULL DEFAULT 0;

COMMENT ON COLUMN templates.restart_requirement_days_of_week IS 'A bitmap of days of week to restart the workspace on, starting with Monday as the 0th bit, and Sunday as the 6th bit. The 7th bit is unused.';
COMMENT ON COLUMN templates.restart_requirement_weeks IS 'The number of weeks between restarts. 0 weeks means "every week", 1 week means "every other week", etc. Weeks are counted from January 2, 2023, which is the first Monday of 2023. This is to ensure workspaces are started consistently for all customers on the same n-week cycles.';

COMMIT;
