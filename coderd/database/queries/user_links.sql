-- name: GetUserLinkByLinkedID :one
SELECT
	user_links.*
FROM
	user_links
INNER JOIN
	users ON user_links.user_id = users.id
WHERE
	linked_id = $1
	AND
	deleted = false;

-- name: GetUserLinkByUserIDLoginType :one
SELECT
	*
FROM
	user_links
WHERE
	user_id = $1 AND login_type = $2;

-- name: GetUserLinksByUserID :many
SELECT * FROM user_links WHERE user_id = $1;

-- name: InsertUserLink :one
INSERT INTO
	user_links (
		user_id,
		login_type,
		linked_id,
		oauth_access_token,
		oauth_access_token_key_id,
		oauth_refresh_token,
		oauth_refresh_token_key_id,
		oauth_expiry,
		claims
	)
VALUES
	( $1, $2, $3, $4, $5, $6, $7, $8, $9 ) RETURNING *;

-- name: UpdateUserLink :one
UPDATE
	user_links
SET
	oauth_access_token = $1,
	oauth_access_token_key_id = $2,
	oauth_refresh_token = $3,
	oauth_refresh_token_key_id = $4,
	oauth_expiry = $5,
	claims = $6
WHERE
	user_id = $7 AND login_type = $8 RETURNING *;

-- name: UpdateUserLinkedID :one
-- Backfills linked_id for legacy user_links that were created before
-- linked_id tracking was added. Only updates when linked_id is empty
-- to avoid overwriting a valid binding.
UPDATE
	user_links
SET
	linked_id = @linked_id
WHERE
	user_id = @user_id AND login_type = @login_type AND linked_id = '' RETURNING *;

-- name: OIDCClaimFields :many
-- OIDCClaimFields returns a list of distinct keys in the the merged_claims fields.
-- This query is used to generate the list of available sync fields for idp sync settings.
SELECT
	DISTINCT jsonb_object_keys(claims->'merged_claims')
FROM
	user_links
WHERE
    -- Only return rows where the top level key exists
	claims ? 'merged_claims' AND
    -- 'null' is the default value for the id_token_claims field
	-- jsonb 'null' is not the same as SQL NULL. Strip these out.
	jsonb_typeof(claims->'merged_claims') != 'null' AND
	login_type = 'oidc'
	AND CASE WHEN @organization_id :: uuid != '00000000-0000-0000-0000-000000000000'::uuid  THEN
		user_links.user_id = ANY(SELECT organization_members.user_id FROM organization_members WHERE organization_id = @organization_id)
		ELSE true
	END
;

-- name: OIDCClaimFieldValues :many
SELECT
	-- DISTINCT to remove duplicates
	DISTINCT jsonb_array_elements_text(CASE
		-- When the type is an array, filter out any non-string elements.
		-- This is to keep the return type consistent.
		WHEN jsonb_typeof(claims->'merged_claims'->sqlc.arg('claim_field')::text) = 'array' THEN
			(
				SELECT
					jsonb_agg(element)
				FROM
					jsonb_array_elements(claims->'merged_claims'->sqlc.arg('claim_field')::text) AS element
				WHERE
					-- Filtering out non-string elements
					jsonb_typeof(element) = 'string'
			)
		-- Some IDPs return a single string instead of an array of strings.
		WHEN jsonb_typeof(claims->'merged_claims'->sqlc.arg('claim_field')::text) = 'string' THEN
			jsonb_build_array(claims->'merged_claims'->sqlc.arg('claim_field')::text)
	END)
FROM
	user_links
WHERE
	-- IDP sync only supports string and array (of string) types
	jsonb_typeof(claims->'merged_claims'->sqlc.arg('claim_field')::text) = ANY(ARRAY['string', 'array'])
	AND login_type = 'oidc'
	AND CASE
		WHEN @organization_id :: uuid != '00000000-0000-0000-0000-000000000000'::uuid  THEN
			user_links.user_id = ANY(SELECT organization_members.user_id FROM organization_members WHERE organization_id = @organization_id)
		ELSE true
	END
;

-- name: CountOIDCLinkedIDsByIssuer :many
-- Groups OIDC user links by their issuer prefix (the part before "||" in
-- linked_id) and returns a count for each. Empty linked_ids are reported
-- with an empty issuer_prefix. Used for analysis before resetting
-- mismatched links.
SELECT
	(CASE
		WHEN user_links.linked_id = '' THEN ''
		ELSE split_part(user_links.linked_id, '||', 1)
	END)::text AS issuer_prefix,
	COUNT(*)::int AS count
FROM
	user_links
INNER JOIN
	users ON user_links.user_id = users.id
WHERE
	user_links.login_type = 'oidc'
	AND users.deleted = false
GROUP BY issuer_prefix;

-- name: UnlinkOIDCUsersByIssuerMismatch :execrows
-- Resets linked_id to '' for OIDC links where the linked_id is non-empty
-- and does not begin with the expected issuer prefix. This allows users to
-- re-authenticate under a new OIDC provider.
UPDATE user_links
SET linked_id = ''
FROM users
WHERE user_links.user_id = users.id
	AND user_links.login_type = 'oidc'
	AND user_links.linked_id != ''
	AND NOT starts_with(user_links.linked_id, @expected_prefix)
	AND users.deleted = false;
