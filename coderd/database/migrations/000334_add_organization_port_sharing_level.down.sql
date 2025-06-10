-- Update columns to use the old enum, replacing 'organization' with 'owner'
ALTER TABLE workspace_agent_port_share
	ALTER COLUMN share_level TYPE app_sharing_level USING (
		CASE
			WHEN share_level = 'organization' THEN 'owner'::app_sharing_level
			ELSE share_level::text::app_sharing_level
		END
	);


-- Drop new enum
DROP TYPE port_sharing_level;
