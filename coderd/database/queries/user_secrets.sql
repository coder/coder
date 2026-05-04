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
-- distribution is active non-system users. Specifically:
--
--   * deleted = false: Coder soft-deletes by flipping users.deleted
--     rather than removing rows, so secrets persist after delete but
--     are unreachable.
--   * status = 'active': dormant users (no recent activity) and
--     suspended users (explicitly disabled) cannot use secrets, so
--     they shouldn't dilute the percentile distribution as
--     zero-secret entries.
--   * is_system = false: internal subjects like the prebuilds user
--     never use secrets in the normal flow.
--
-- Status transitions move users in and out of this denominator, so a
-- snapshot's UsersWithSecrets can drop without any secret being
-- deleted.
--
-- The percentile distribution is computed across all active non-system
-- users, including those with zero secrets, so the percentiles reflect
-- deployment-wide adoption rather than only the power-user subset.
-- percentile_disc returns an actual integer count from the underlying
-- values rather than interpolating between rows.
WITH active_users AS (
    SELECT id AS user_id
    FROM users
    WHERE deleted = false
      AND is_system = false
      AND status = 'active'::user_status
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
