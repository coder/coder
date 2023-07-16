BEGIN;

ALTER TABLE templates
	DROP COLUMN restart_requirement_days_of_week,
	DROP COLUMN restart_requirement_weeks;

ALTER TABLE users DROP COLUMN quiet_hours_schedule;

COMMIT;
