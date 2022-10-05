-- Add enum app_share_level
CREATE TYPE app_share_level AS ENUM (
	-- only the workspace owner can access the app
	'owner',
	-- the workspace owner and other users that can read the workspace template
	-- can access the app
	'template',
	-- any authenticated user on the site can access the app
	'authenticated',
	-- any user can access the app even if they are not authenticated
	'public'
);

-- Add share_level column to workspace_apps table
ALTER TABLE workspace_apps ADD COLUMN share_level app_share_level NOT NULL DEFAULT 'owner'::app_share_level;
