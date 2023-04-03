ALTER TABLE "templates"
	ADD COLUMN "allow_user_auto_start" boolean DEFAULT true NOT NULL,
	ADD COLUMN "allow_user_auto_stop" boolean DEFAULT true NOT NULL;

COMMENT ON COLUMN "templates"."allow_user_auto_start"
	IS 'Allow users to specify an auto-start schedule for workspaces (enterprise).';

COMMENT ON COLUMN "templates"."allow_user_auto_stop"
	IS 'Allow users to specify custom auto-stop values for workspaces (enterprise).';
