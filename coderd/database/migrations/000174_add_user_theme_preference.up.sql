ALTER TABLE users ADD COLUMN "theme_preference" text;

COMMENT ON COLUMN "users"."theme_preference" IS 'null can be interpreted as "unset", defaulting to the operators default theme';
