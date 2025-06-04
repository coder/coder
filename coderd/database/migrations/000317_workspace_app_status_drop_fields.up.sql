ALTER TABLE ONLY workspace_app_statuses
	DROP COLUMN IF EXISTS needs_user_attention,
	DROP COLUMN IF EXISTS icon;
