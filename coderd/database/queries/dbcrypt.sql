-- name: GetDBCryptKeys :many
SELECT * FROM dbcrypt_keys ORDER BY number ASC;

-- name: RevokeDBCryptKey :exec
UPDATE dbcrypt_keys
SET
	revoked_key_digest = active_key_digest,
	active_key_digest = revoked_key_digest,
	revoked_at = CURRENT_TIMESTAMP
WHERE
	active_key_digest = @active_key_digest::text
AND
	revoked_key_digest IS NULL;

-- name: InsertDBCryptKey :exec
INSERT INTO dbcrypt_keys
	(number, active_key_digest, created_at, test)
VALUES (@number::int, @active_key_digest::text, CURRENT_TIMESTAMP, @test::text);

