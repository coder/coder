ALTER TABLE "templates"
	ADD COLUMN "allow_user_autostart" boolean DEFAULT true NOT NULL,
	ADD COLUMN "allow_user_autostop" boolean DEFAULT true NOT NULL;

COMMENT ON COLUMN "templates"."allow_user_autostart"
	IS 'Allow users to specify an autostart schedule for workspaces (enterprise).';

COMMENT ON COLUMN "templates"."allow_user_autostop"
	IS 'Allow users to specify custom autostop values for workspaces (enterprise).';
