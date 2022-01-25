-- Database queries are generated using sqlc. See:
-- https://docs.sqlc.dev/en/latest/tutorials/getting-started-postgresql.html
--
-- Run "make gen" to generate models and query functions.
;

-- name: GetAPIKeyByID :one
SELECT
  *
FROM
  api_keys
WHERE
  id = $1
LIMIT
  1;

-- name: GetUserByID :one
SELECT
  *
FROM
  users
WHERE
  id = $1
LIMIT
  1;

-- name: GetUserByEmailOrUsername :one
SELECT
  *
FROM
  users
WHERE
  LOWER(username) = LOWER(@username)
  OR email = @email
LIMIT
  1;

-- name: GetUserCount :one
SELECT
  COUNT(*)
FROM
  users;

-- name: GetOrganizationByName :one
SELECT
  *
FROM
  organizations
WHERE
  LOWER(name) = LOWER(@name)
LIMIT
  1;

-- name: GetOrganizationsByUserID :many
SELECT
  *
FROM
  organizations
WHERE
  id = (
    SELECT
      organization_id
    FROM
      organization_members
    WHERE
      user_id = $1
  );

-- name: GetOrganizationMemberByUserID :one
SELECT
  *
FROM
  organization_members
WHERE
  organization_id = $1
  AND user_id = $2
LIMIT
  1;

-- name: GetProjectByID :one
SELECT
  *
FROM
  project
WHERE
  id = $1
LIMIT
  1;

-- name: GetProjectByOrganizationAndName :one
SELECT
  *
FROM
  project
WHERE
  organization_id = @organization_id
  AND LOWER(name) = LOWER(@name)
LIMIT
  1;

-- name: GetProjectsByOrganizationIDs :many
SELECT
  *
FROM
  project
WHERE
  organization_id = ANY(@ids :: text [ ]);

-- name: GetProjectHistoryByProjectID :many
SELECT
  *
FROM
  project_history
WHERE
  project_id = $1;

-- name: GetProjectHistoryByID :one
SELECT
  *
FROM
  project_history
WHERE
  id = $1;

-- name: GetWorkspacesByUserID :many
SELECT
  *
FROM
  workspace
WHERE
  owner_id = $1;

-- name: GetWorkspaceByUserIDAndName :one
SELECT
  *
FROM
  workspace
WHERE
  owner_id = @owner_id
  AND LOWER(name) = LOWER(@name);

-- name: GetWorkspacesByProjectAndUserID :many
SELECT
  *
FROM
  workspace
WHERE
  owner_id = $1
  AND project_id = $2;

-- name: GetWorkspaceHistoryByWorkspaceID :many
SELECT
  *
FROM
  workspace_history
WHERE
  workspace_id = $1;

-- name: GetWorkspaceHistoryByWorkspaceIDWithoutAfter :one
SELECT
  *
FROM
  workspace_history
WHERE
  workspace_id = $1
  AND after_id IS NULL
LIMIT
  1;

-- name: GetWorkspaceResourcesByHistoryID :many
SELECT
  *
FROM
  workspace_resource
WHERE
  workspace_history_id = $1;

-- name: GetWorkspaceAgentsByResourceIDs :many
SELECT
  *
FROM
  workspace_agent
WHERE
  workspace_resource_id = ANY(@ids :: uuid [ ]);

-- name: InsertAPIKey :one
INSERT INTO
  api_keys (
    id,
    hashed_secret,
    user_id,
    application,
    name,
    last_used,
    expires_at,
    created_at,
    updated_at,
    login_type,
    oidc_access_token,
    oidc_refresh_token,
    oidc_id_token,
    oidc_expiry,
    devurl_token
  )
VALUES
  (
    $1,
    $2,
    $3,
    $4,
    $5,
    $6,
    $7,
    $8,
    $9,
    $10,
    $11,
    $12,
    $13,
    $14,
    $15
  ) RETURNING *;

-- name: InsertOrganization :one
INSERT INTO
  organizations (id, name, description, created_at, updated_at)
VALUES
  ($1, $2, $3, $4, $5) RETURNING *;

-- name: InsertOrganizationMember :one
INSERT INTO
  organization_members (
    organization_id,
    user_id,
    created_at,
    updated_at,
    roles
  )
VALUES
  ($1, $2, $3, $4, $5) RETURNING *;

-- name: InsertProject :one
INSERT INTO
  project (
    id,
    created_at,
    updated_at,
    organization_id,
    name,
    provisioner
  )
VALUES
  ($1, $2, $3, $4, $5, $6) RETURNING *;

-- name: InsertProjectHistory :one
INSERT INTO
  project_history (
    id,
    project_id,
    created_at,
    updated_at,
    name,
    description,
    storage_method,
    storage_source,
    import_job_id
  )
VALUES
  ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING *;

-- name: InsertProjectParameter :one
INSERT INTO
  project_parameter (
    id,
    created_at,
    project_history_id,
    name,
    description,
    default_source,
    allow_override_source,
    default_destination,
    allow_override_destination,
    default_refresh,
    redisplay_value,
    validation_error,
    validation_condition,
    validation_type_system,
    validation_value_type
  )
VALUES
  (
    $1,
    $2,
    $3,
    $4,
    $5,
    $6,
    $7,
    $8,
    $9,
    $10,
    $11,
    $12,
    $13,
    $14,
    $15
  ) RETURNING *;

-- name: InsertUser :one
INSERT INTO
  users (
    id,
    email,
    name,
    login_type,
    revoked,
    hashed_password,
    created_at,
    updated_at,
    username
  )
VALUES
  ($1, $2, $3, $4, false, $5, $6, $7, $8) RETURNING *;

-- name: InsertWorkspace :one
INSERT INTO
  workspace (
    id,
    created_at,
    updated_at,
    owner_id,
    project_id,
    name
  )
VALUES
  ($1, $2, $3, $4, $5, $6) RETURNING *;

-- name: InsertWorkspaceAgent :one
INSERT INTO
  workspace_agent (
    id,
    workspace_resource_id,
    created_at,
    updated_at,
    instance_metadata,
    resource_metadata
  )
VALUES
  ($1, $2, $3, $4, $5, $6) RETURNING *;

-- name: InsertWorkspaceHistory :one
INSERT INTO
  workspace_history (
    id,
    created_at,
    updated_at,
    workspace_id,
    project_history_id,
    before_id,
    transition,
    initiator,
    provision_job_id
  )
VALUES
  ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING *;

-- name: InsertWorkspaceResource :one
INSERT INTO
  workspace_resource (
    id,
    created_at,
    workspace_history_id,
    type,
    name,
    workspace_agent_token
  )
VALUES
  ($1, $2, $3, $4, $5, $6) RETURNING *;

-- name: UpdateAPIKeyByID :exec
UPDATE
  api_keys
SET
  last_used = $2,
  expires_at = $3,
  oidc_access_token = $4,
  oidc_refresh_token = $5,
  oidc_expiry = $6
WHERE
  id = $1;

-- name: UpdateWorkspaceHistoryByID :exec
UPDATE
  workspace_history
SET
  updated_at = $2,
  after_id = $3
WHERE
  id = $1;
