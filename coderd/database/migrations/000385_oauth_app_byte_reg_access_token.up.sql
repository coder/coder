
-- Convert registration_access_token column from text to bytea in oauth2_provider_apps table
-- This feature is in experimental stage, and not currently used outside dogfood.
--
-- The PR alongside this migration makes all the current apps invalid, effectivity breaking them
ALTER TABLE oauth2_provider_apps
	ALTER COLUMN registration_access_token
		SET DATA TYPE bytea
		USING decode(registration_access_token, 'escape');
