UPDATE "users"
	SET "avatar_url" = ''
	WHERE "avatar_url" IS NULL;

ALTER TABLE "users"
	ALTER COLUMN "avatar_url" SET NOT NULL,
	ALTER COLUMN "avatar_url" SET DEFAULT '';
