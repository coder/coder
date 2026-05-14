ALTER TABLE external_auth_links
	ADD COLUMN external_user_id text NOT NULL DEFAULT '',
	ADD COLUMN external_user_login text NOT NULL DEFAULT '',
	ADD COLUMN external_user_name text NOT NULL DEFAULT '',
	ADD COLUMN external_user_email text NOT NULL DEFAULT '',
	ADD COLUMN external_user_avatar_url text NOT NULL DEFAULT '';

CREATE UNIQUE INDEX external_auth_links_provider_external_user_id_idx
	ON external_auth_links (provider_id, external_user_id)
	WHERE external_user_id != '';

DELETE FROM external_auth_links
WHERE user_id IN (SELECT id FROM users WHERE deleted);

CREATE OR REPLACE FUNCTION delete_deleted_user_resources() RETURNS trigger
	LANGUAGE plpgsql
AS $$
DECLARE
BEGIN
	IF (NEW.deleted) THEN
		-- Remove their api_keys
		DELETE FROM api_keys
		WHERE user_id = OLD.id;

		-- Remove their user_links
		-- Their login_type is preserved in the users table.
		-- Matching this user back to the link can still be done by their
		-- email if the account is undeleted. Although that is not a guarantee.
		DELETE FROM user_links
		WHERE user_id = OLD.id;

		-- Remove their external_auth_links.
		DELETE FROM external_auth_links
		WHERE user_id = OLD.id;

		-- Remove their user_secrets.
		-- user_secrets.user_id has ON DELETE CASCADE, but soft-delete
		-- does not remove the users row so the FK cascade never fires.
		DELETE FROM user_secrets
		WHERE user_id = OLD.id;
	END IF;
	RETURN NEW;
END;
$$;
