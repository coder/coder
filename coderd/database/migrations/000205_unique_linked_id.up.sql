-- Remove the linked_id if two user_links share the same value.
-- This will affect the user if they attempt to change their settings on
-- the oauth/oidc provider. However, if two users exist with the same
-- linked_value, there is no way to determine correctly which user should
-- be updated. Since the linked_id is empty, this value will be linked
-- by email.
UPDATE ONLY user_links AS out
SET
	linked_id =
		CASE WHEN (
			  -- When the count of linked_id is greater than 1, set the linked_id to empty
			  SELECT
			      COUNT(*)
			  FROM
			      user_links inn
			  WHERE
			      out.linked_id = inn.linked_id AND out.login_type = inn.login_type
		  ) > 1 THEN '' ELSE out.linked_id END;

-- Enforce unique linked_id constraint on non-empty linked_id
CREATE UNIQUE INDEX user_links_linked_id_login_type_idx ON user_links USING btree (linked_id, login_type) WHERE (linked_id != '');
