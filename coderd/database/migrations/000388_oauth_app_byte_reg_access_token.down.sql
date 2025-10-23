ALTER TABLE oauth2_provider_apps
	ALTER COLUMN registration_access_token
		SET DATA TYPE text
		USING encode(registration_access_token, 'escape');
