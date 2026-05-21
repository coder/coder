ALTER TABLE users ADD COLUMN "theme_preference" text NOT NULL DEFAULT '';

COMMENT ON COLUMN "users"."theme_preference" IS '"" can be interpreted as "the user does not care", falling back to the default theme';
