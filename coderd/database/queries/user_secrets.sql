-- name: GetUserSecretByUserIDAndName :one
SELECT *
FROM user_secrets
WHERE user_id = @user_id AND name = @name;

-- name: GetUserSecretByID :one
SELECT *
FROM user_secrets
WHERE id = @id;

-- name: ListUserSecrets :many
-- Returns metadata only (no value or value_key_id) for the
-- REST API list and get endpoints.
SELECT
    id, user_id, name, description,
    env_name, file_path,
    created_at, updated_at
FROM user_secrets
WHERE user_id = @user_id
ORDER BY name ASC;

-- name: ListUserSecretsWithValues :many
-- Returns all columns including the secret value. Used by the
-- provisioner (build-time injection) and the agent manifest
-- (runtime injection).
SELECT *
FROM user_secrets
WHERE user_id = @user_id
ORDER BY name ASC;

-- name: CreateUserSecret :one
INSERT INTO user_secrets (
    id,
    user_id,
    name,
    description,
    value,
    value_key_id,
    env_name,
    file_path
) VALUES (
    @id,
    @user_id,
    @name,
    @description,
    @value,
    @value_key_id,
    @env_name,
    @file_path
) RETURNING *;

-- name: UpdateUserSecretByUserIDAndName :one
UPDATE user_secrets
SET
    value       = CASE WHEN @update_value::bool THEN @value ELSE value END,
    value_key_id = CASE WHEN @update_value::bool THEN @value_key_id ELSE value_key_id END,
    description = CASE WHEN @update_description::bool THEN @description ELSE description END,
    env_name    = CASE WHEN @update_env_name::bool THEN @env_name ELSE env_name END,
    file_path   = CASE WHEN @update_file_path::bool THEN @file_path ELSE file_path END,
    updated_at  = CURRENT_TIMESTAMP
WHERE user_id = @user_id AND name = @name
RETURNING *;

-- name: DeleteUserSecretByUserIDAndName :one
DELETE FROM user_secrets
WHERE user_id = @user_id AND name = @name
RETURNING *;

-- name: GetUserSecretsTelemetrySummary :one
-- Returns deployment-wide aggregates for the telemetry snapshot.
--
-- The denominator for both user-level counts and the per-user
-- distribution is active non-system users. Soft-deleted users are
-- excluded because Coder soft-deletes by flipping users.deleted
-- rather than removing rows, so their secrets persist in user_secrets
-- but are no longer reachable. System users (is_system = true) cover
-- internal subjects like the prebuilds user that never use secrets.
--
-- The percentile distribution is computed across all active non-system
-- users, including those with zero secrets, so the percentiles reflect
-- deployment-wide adoption rather than only the power-user subset.
-- percentile_disc returns an actual integer count from the underlying
-- values rather than interpolating between rows.
WITH active_users AS (
    SELECT id AS user_id
    FROM users
    WHERE deleted = false AND is_system = false
),
per_user AS (
    SELECT au.user_id, COUNT(us.id)::bigint AS n
    FROM active_users au
    LEFT JOIN user_secrets us ON us.user_id = au.user_id
    GROUP BY au.user_id
),
secrets_filtered AS (
    SELECT us.env_name, us.file_path
    FROM user_secrets us
    JOIN active_users au ON au.user_id = us.user_id
)
SELECT
    COUNT(*) FILTER (WHERE n > 0)::bigint                                                       AS users_with_secrets,
    (SELECT COUNT(*) FROM secrets_filtered)::bigint                                             AS total_secrets,
    (SELECT COUNT(*) FROM secrets_filtered WHERE env_name != '' AND file_path = '' )::bigint    AS env_name_only,
    (SELECT COUNT(*) FROM secrets_filtered WHERE env_name = ''  AND file_path != '')::bigint    AS file_path_only,
    (SELECT COUNT(*) FROM secrets_filtered WHERE env_name != '' AND file_path != '')::bigint    AS both,
    (SELECT COUNT(*) FROM secrets_filtered WHERE env_name = ''  AND file_path = '' )::bigint    AS neither,
    COALESCE(MAX(n), 0)::bigint                                                                 AS secrets_per_user_max,
    COALESCE(percentile_disc(0.25) WITHIN GROUP (ORDER BY n), 0)::bigint                        AS secrets_per_user_p25,
    COALESCE(percentile_disc(0.50) WITHIN GROUP (ORDER BY n), 0)::bigint                        AS secrets_per_user_p50,
    COALESCE(percentile_disc(0.75) WITHIN GROUP (ORDER BY n), 0)::bigint                        AS secrets_per_user_p75,
    COALESCE(percentile_disc(0.90) WITHIN GROUP (ORDER BY n), 0)::bigint                        AS secrets_per_user_p90
FROM per_user;
