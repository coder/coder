BEGIN;

ALTER TABLE templates RENAME COLUMN autostop_requirement_days_of_week TO restart_requirement_days_of_week;

ALTER TABLE templates RENAME COLUMN autostop_requirement_weeks TO restart_requirement_weeks;

COMMIT;
