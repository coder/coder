ALTER TABLE users ADD COLUMN "theme_preference" text;

COMMENT ON COLUMN "users"."theme_preference" IS 'null can be interpreted as "the user does not care", falling back to the default theme';
