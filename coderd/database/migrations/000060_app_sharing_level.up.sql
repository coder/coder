-- Add enum app_sharing_level
CREATE TYPE app_sharing_level AS ENUM (
	-- only the workspace owner can access the app
	'owner',
	-- any authenticated user on the site can access the app
	'authenticated',
	-- any user can access the app even if they are not authenticated
	'public'
);

-- Add sharing_level column to workspace_apps table
ALTER TABLE workspace_apps ADD COLUMN sharing_level app_sharing_level NOT NULL DEFAULT 'owner'::app_sharing_level;
