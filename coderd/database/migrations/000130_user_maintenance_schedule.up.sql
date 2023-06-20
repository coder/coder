-- empty schedule means use the default if entitled
ALTER TABLE users ADD COLUMN maintenance_schedule text NOT NULL DEFAULT '';
