ALTER TABLE templates
	DROP COLUMN restart_requirement_days_of_week,
	DROP COLUMN restart_requirement_weeks,
	ADD COLUMN max_ttl bigint NOT NULL DEFAULT 0;
