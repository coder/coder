-- Database queries are generated using sqlc. See:
-- https://docs.sqlc.dev/en/latest/tutorials/getting-started-postgresql.html
--
-- Run "make gen" to generate models and query functions.
;

-- Acquires the lock for a single job that isn't started, completed,
-- cancelled, and that matches an array of provisioner types.
--
-- SKIP LOCKED is used to jump over locked rows. This prevents
-- multiple provisioners from acquiring the same jobs. See:
-- https://www.postgresql.org/docs/9.5/sql-select.html#SQL-FOR-UPDATE-SHARE
-- name: AcquireProvisionerJob :one
UPDATE
  provisioner_job
SET
  started_at = @started_at,
  updated_at = @started_at,
  worker_id = @worker_id
WHERE
  id = (
    SELECT
      id
    FROM
      provisioner_job AS nested
    WHERE
      nested.started_at IS NULL
      AND nested.cancelled_at IS NULL
      AND nested.completed_at IS NULL
      AND nested.provisioner = ANY(@types :: provisioner_type [ ])
    ORDER BY
      nested.created_at FOR
    UPDATE
      SKIP LOCKED
    LIMIT
      1
  ) RETURNING *;

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

-- name: GetOrganizationByID :one
SELECT
  *
FROM
  organizations
WHERE
  id = $1;

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

-- name: GetParameterValuesByScope :many
SELECT
  *
FROM
  parameter_value
WHERE
  scope = $1
  AND scope_id = $2;

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

-- name: GetProjectParametersByHistoryID :many
SELECT
  *
FROM
  project_parameter
WHERE
  project_history_id = $1;

-- name: GetProjectHistoryByProjectID :many
SELECT
  *
FROM
  project_history
WHERE
  project_id = $1;

-- name: GetProjectHistoryByProjectIDAndName :one
SELECT
  *
FROM
  project_history
WHERE
  project_id = $1
  AND name = $2;

-- name: GetProjectHistoryByID :one
SELECT
  *
FROM
  project_history
WHERE
  id = $1;

-- name: GetProjectHistoryLogsByIDBetween :many
SELECT
  *
FROM
  project_history_log
WHERE
  project_history_id = @project_history_id
  AND (
    created_at >= @created_after
    OR created_at <= @created_before
  )
ORDER BY
  created_at;

-- name: GetProvisionerDaemons :many
SELECT
  *
FROM
  provisioner_daemon;

-- name: GetProvisionerDaemonByID :one
SELECT
  *
FROM
  provisioner_daemon
WHERE
  id = $1;

-- name: GetProvisionerJobByID :one
SELECT
  *
FROM
  provisioner_job
WHERE
  id = $1;

-- name: GetWorkspaceByID :one
SELECT
  *
FROM
  workspace
WHERE
  id = $1
LIMIT
  1;

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

-- name: GetWorkspaceHistoryByID :one
SELECT
  *
FROM
  workspace_history
WHERE
  id = $1
LIMIT
  1;

-- name: GetWorkspaceHistoryByWorkspaceIDAndName :one
SELECT
  *
FROM
  workspace_history
WHERE
  workspace_id = $1
  AND name = $2;

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

-- name: GetWorkspaceHistoryLogsByIDBetween :many
SELECT
  *
FROM
  workspace_history_log
WHERE
  workspace_history_id = @workspace_history_id
  AND (
    created_at >= @created_after
    OR created_at <= @created_before
  )
ORDER BY
  created_at;

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

-- name: InsertParameterValue :one
INSERT INTO
  parameter_value (
    id,
    name,
    created_at,
    updated_at,
    scope,
    scope_id,
    source_scheme,
    source_value,
    destination_scheme,
    destination_value
  )
VALUES
  ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) RETURNING *;

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

-- name: InsertProjectHistoryLogs :many
INSERT INTO
  project_history_log
SELECT
  @project_history_id :: uuid AS project_history_id,
  unnest(@id :: uuid [ ]) AS id,
  unnest(@created_at :: timestamptz [ ]) AS created_at,
  unnest(@source :: log_source [ ]) as source,
  unnest(@level :: log_level [ ]) as level,
  unnest(@output :: varchar(1024) [ ]) as output RETURNING *;

-- name: InsertProjectParameter :one
INSERT INTO
  project_parameter (
    id,
    created_at,
    project_history_id,
    name,
    description,
    default_source_scheme,
    default_source_value,
    allow_override_source,
    default_destination_scheme,
    default_destination_value,
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
    $15,
    $16,
    $17
  ) RETURNING *;

-- name: InsertProvisionerDaemon :one
INSERT INTO
  provisioner_daemon (id, created_at, name, provisioners)
VALUES
  ($1, $2, $3, $4) RETURNING *;

-- name: InsertProvisionerJob :one
INSERT INTO
  provisioner_job (
    id,
    created_at,
    updated_at,
    initiator_id,
    provisioner,
    type,
    project_id,
    input
  )
VALUES
  ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING *;

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
    name,
    transition,
    initiator,
    provision_job_id,
    provisioner_state
  )
VALUES
  ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) RETURNING *;

-- name: InsertWorkspaceHistoryLogs :many
INSERT INTO
  workspace_history_log
SELECT
  unnest(@id :: uuid [ ]) AS id,
  @workspace_history_id :: uuid AS workspace_history_id,
  unnest(@created_at :: timestamptz [ ]) AS created_at,
  unnest(@source :: log_source [ ]) as source,
  unnest(@level :: log_level [ ]) as level,
  unnest(@output :: varchar(1024) [ ]) as output RETURNING *;

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

-- name: UpdateProvisionerDaemonByID :exec
UPDATE
  provisioner_daemon
SET
  updated_at = $2,
  provisioners = $3
WHERE
  id = $1;

-- name: UpdateProvisionerJobByID :exec
UPDATE
  provisioner_job
SET
  updated_at = $2
WHERE
  id = $1;

-- name: UpdateProvisionerJobWithCompleteByID :exec
UPDATE
  provisioner_job
SET
  updated_at = $2,
  completed_at = $3,
  cancelled_at = $4,
  error = $5
WHERE
  id = $1;

-- name: UpdateWorkspaceHistoryByID :exec
UPDATE
  workspace_history
SET
  updated_at = $2,
  after_id = $3,
  provisioner_state = $4
WHERE
  id = $1;
