ALTER TABLE oauth2_provider_apps
	ALTER COLUMN registration_access_token
		SET DATA TYPE bytea
		USING decode(registration_access_token, 'escape');
