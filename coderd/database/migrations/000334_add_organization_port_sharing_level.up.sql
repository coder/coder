-- Add 'organization' to the app_sharing_level enum
CREATE TYPE port_sharing_level AS ENUM (
	'owner',
	'authenticated',
	'organization',
	'public'
);


-- Update columns to use the new enum
ALTER TABLE workspace_agent_port_share
	ALTER COLUMN share_level TYPE port_sharing_level USING (share_level::text::port_sharing_level);
