-- empty schedule means use the default if entitled
ALTER TABLE users ADD COLUMN quiet_hours_schedule text NOT NULL DEFAULT '';
