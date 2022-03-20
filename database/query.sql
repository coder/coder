-- Database queries are generated using sqlc. See:
-- https://docs.sqlc.dev/en/latest/tutorials/getting-started-postgresql.html
--
-- Run "make gen" to generate models and query functions.
;

-- Acquires the lock for a single job that isn't started, completed,
-- canceled, and that matches an array of provisioner types.
--
-- SKIP LOCKED is used to jump over locked rows. This prevents
-- multiple provisioners from acquiring the same jobs. See:
-- https://www.postgresql.org/docs/9.5/sql-select.html#SQL-FOR-UPDATE-SHARE
-- name: AcquireProvisionerJob :one
UPDATE
  provisioner_jobs
SET
  started_at = @started_at,
  updated_at = @started_at,
  worker_id = @worker_id
WHERE
  id = (
    SELECT
      id
    FROM
      provisioner_jobs AS nested
    WHERE
      nested.started_at IS NULL
      AND nested.canceled_at IS NULL
      AND nested.completed_at IS NULL
      AND nested.provisioner = ANY(@types :: provisioner_type [ ])
    ORDER BY
      nested.created_at FOR
    UPDATE
      SKIP LOCKED
    LIMIT
      1
  ) RETURNING *;

-- name: DeleteParameterValueByID :exec
DELETE FROM
  parameter_values
WHERE
  id = $1;

-- name: GetAPIKeyByID :one
SELECT
  *
FROM
  api_keys
WHERE
  id = $1
LIMIT
  1;

-- name: GetFileByHash :one
SELECT
  *
FROM
  files
WHERE
  hash = $1
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
  parameter_values
WHERE
  scope = $1
  AND scope_id = $2;

-- name: GetParameterValueByScopeAndName :one
SELECT
  *
FROM
  parameter_values
WHERE
  scope = $1
  AND scope_id = $2
  AND name = $3
LIMIT
  1;

-- name: GetProjectByID :one
SELECT
  *
FROM
  projects
WHERE
  id = $1
LIMIT
  1;

-- name: GetProjectsByIDs :many
SELECT
  *
FROM
  projects
WHERE
  id = ANY(@ids :: uuid [ ]);

-- name: GetProjectByOrganizationAndName :one
SELECT
  *
FROM
  projects
WHERE
  organization_id = @organization_id
  AND deleted = @deleted
  AND LOWER(name) = LOWER(@name)
LIMIT
  1;

-- name: GetProjectsByOrganization :many
SELECT
  *
FROM
  projects
WHERE
  organization_id = $1
  AND deleted = $2;

-- name: GetParameterSchemasByJobID :many
SELECT
  *
FROM
  parameter_schemas
WHERE
  job_id = $1;

-- name: GetProjectVersionsByProjectID :many
SELECT
  *
FROM
  project_versions
WHERE
  project_id = $1 :: uuid;

-- name: GetProjectVersionByJobID :one
SELECT
  *
FROM
  project_versions
WHERE
  job_id = $1;

-- name: GetProjectVersionByProjectIDAndName :one
SELECT
  *
FROM
  project_versions
WHERE
  project_id = $1
  AND name = $2;

-- name: GetProjectVersionByID :one
SELECT
  *
FROM
  project_versions
WHERE
  id = $1;

-- name: GetProvisionerLogsByIDBetween :many
SELECT
  *
FROM
  provisioner_job_logs
WHERE
  job_id = @job_id
  AND (
    created_at >= @created_after
    OR created_at <= @created_before
  )
ORDER BY
  created_at;

-- name: GetProvisionerDaemonByID :one
SELECT
  *
FROM
  provisioner_daemons
WHERE
  id = $1;

-- name: GetProvisionerDaemons :many
SELECT
  *
FROM
  provisioner_daemons;

-- name: GetWorkspaceAgentByAuthToken :one
SELECT
  *
FROM
  workspace_agents
WHERE
  auth_token = $1
ORDER BY
  created_at DESC;

-- name: GetWorkspaceAgentByInstanceID :one
SELECT
  *
FROM
  workspace_agents
WHERE
  auth_instance_id = @auth_instance_id :: text
ORDER BY
  created_at DESC;

-- name: GetProvisionerJobByID :one
SELECT
  *
FROM
  provisioner_jobs
WHERE
  id = $1;

-- name: GetProvisionerJobsByIDs :many
SELECT
  *
FROM
  provisioner_jobs
WHERE
  id = ANY(@ids :: uuid [ ]);

-- name: GetWorkspaceByID :one
SELECT
  *
FROM
  workspaces
WHERE
  id = $1
LIMIT
  1;

-- name: GetWorkspacesByProjectID :many
SELECT
  *
FROM
  workspaces
WHERE
  project_id = $1
  AND deleted = $2;

-- name: GetWorkspacesByUserID :many
SELECT
  *
FROM
  workspaces
WHERE
  owner_id = $1
  AND deleted = $2;

-- name: GetWorkspaceByUserIDAndName :one
SELECT
  *
FROM
  workspaces
WHERE
  owner_id = @owner_id
  AND deleted = @deleted
  AND LOWER(name) = LOWER(@name);

-- name: GetWorkspaceOwnerCountsByProjectIDs :many
SELECT
  project_id,
  COUNT(DISTINCT owner_id)
FROM
  workspaces
WHERE
  project_id = ANY(@ids :: uuid [ ])
GROUP BY
  project_id,
  owner_id;

-- name: GetWorkspaceBuildByID :one
SELECT
  *
FROM
  workspace_builds
WHERE
  id = $1
LIMIT
  1;

-- name: GetWorkspaceBuildByJobID :one
SELECT
  *
FROM
  workspace_builds
WHERE
  job_id = $1
LIMIT
  1;

-- name: GetWorkspaceBuildByWorkspaceIDAndName :one
SELECT
  *
FROM
  workspace_builds
WHERE
  workspace_id = $1
  AND name = $2;

-- name: GetWorkspaceBuildByWorkspaceID :many
SELECT
  *
FROM
  workspace_builds
WHERE
  workspace_id = $1;

-- name: GetWorkspaceBuildByWorkspaceIDWithoutAfter :one
SELECT
  *
FROM
  workspace_builds
WHERE
  workspace_id = $1
  AND after_id IS NULL
LIMIT
  1;

-- name: GetWorkspaceBuildsByWorkspaceIDsWithoutAfter :many
SELECT
  *
FROM
  workspace_builds
WHERE
  workspace_id = ANY(@ids :: uuid [ ])
  AND after_id IS NULL;

-- name: GetWorkspaceResourceByID :one
SELECT
  *
FROM
  workspace_resources
WHERE
  id = $1;

-- name: GetWorkspaceResourcesByJobID :many
SELECT
  *
FROM
  workspace_resources
WHERE
  job_id = $1;

-- name: GetWorkspaceAgentByResourceID :one
SELECT
  *
FROM
  workspace_agents
WHERE
  resource_id = $1;

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

-- name: InsertFile :one
INSERT INTO
  files (hash, created_at, created_by, mimetype, data)
VALUES
  ($1, $2, $3, $4, $5) RETURNING *;

-- name: InsertProvisionerJobLogs :many
INSERT INTO
  provisioner_job_logs
SELECT
  unnest(@id :: uuid [ ]) AS id,
  @job_id :: uuid AS job_id,
  unnest(@created_at :: timestamptz [ ]) AS created_at,
  unnest(@source :: log_source [ ]) as source,
  unnest(@level :: log_level [ ]) as level,
  unnest(@output :: varchar(1024) [ ]) as output RETURNING *;

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
  parameter_values (
    id,
    name,
    created_at,
    updated_at,
    scope,
    scope_id,
    source_scheme,
    source_value,
    destination_scheme
  )
VALUES
  ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING *;

-- name: InsertProject :one
INSERT INTO
  projects (
    id,
    created_at,
    updated_at,
    organization_id,
    name,
    provisioner,
    active_version_id
  )
VALUES
  ($1, $2, $3, $4, $5, $6, $7) RETURNING *;

-- name: InsertWorkspaceResource :one
INSERT INTO
  workspace_resources (
    id,
    created_at,
    job_id,
    transition,
    address,
    type,
    name,
    agent_id
  )
VALUES
  ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING *;

-- name: InsertProjectVersion :one
INSERT INTO
  project_versions (
    id,
    project_id,
    organization_id,
    created_at,
    updated_at,
    name,
    description,
    job_id
  )
VALUES
  ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING *;

-- name: InsertParameterSchema :one
INSERT INTO
  parameter_schemas (
    id,
    created_at,
    job_id,
    name,
    description,
    default_source_scheme,
    default_source_value,
    allow_override_source,
    default_destination_scheme,
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
    $16
  ) RETURNING *;

-- name: InsertProvisionerDaemon :one
INSERT INTO
  provisioner_daemons (id, created_at, organization_id, name, provisioners)
VALUES
  ($1, $2, $3, $4, $5) RETURNING *;

-- name: InsertProvisionerJob :one
INSERT INTO
  provisioner_jobs (
    id,
    created_at,
    updated_at,
    organization_id,
    initiator_id,
    provisioner,
    storage_method,
    storage_source,
    type,
    input
  )
VALUES
  ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) RETURNING *;

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
  workspaces (
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
  workspace_agents (
    id,
    created_at,
    updated_at,
    resource_id,
    auth_token,
    auth_instance_id,
    environment_variables,
    startup_script,
    instance_metadata,
    resource_metadata
  )
VALUES
  ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) RETURNING *;

-- name: InsertWorkspaceBuild :one
INSERT INTO
  workspace_builds (
    id,
    created_at,
    updated_at,
    workspace_id,
    project_version_id,
    before_id,
    name,
    transition,
    initiator,
    job_id,
    provisioner_state
  )
VALUES
  ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) RETURNING *;

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

-- name: UpdateProjectActiveVersionByID :exec
UPDATE
  projects
SET
  active_version_id = $2
WHERE
  id = $1;

-- name: UpdateProjectDeletedByID :exec
UPDATE
  projects
SET
  deleted = $2
WHERE
  id = $1;

-- name: UpdateProjectVersionByID :exec
UPDATE
  project_versions
SET
  project_id = $2,
  updated_at = $3
WHERE
  id = $1;

-- name: UpdateProvisionerDaemonByID :exec
UPDATE
  provisioner_daemons
SET
  updated_at = $2,
  provisioners = $3
WHERE
  id = $1;

-- name: UpdateProvisionerJobByID :exec
UPDATE
  provisioner_jobs
SET
  updated_at = $2
WHERE
  id = $1;

-- name: UpdateProvisionerJobWithCancelByID :exec
UPDATE
  provisioner_jobs
SET
  canceled_at = $2
WHERE
  id = $1;

-- name: UpdateProvisionerJobWithCompleteByID :exec
UPDATE
  provisioner_jobs
SET
  updated_at = $2,
  completed_at = $3,
  canceled_at = $4,
  error = $5
WHERE
  id = $1;

-- name: UpdateWorkspaceDeletedByID :exec
UPDATE
  workspaces
SET
  deleted = $2
WHERE
  id = $1;

-- name: UpdateWorkspaceAgentConnectionByID :exec
UPDATE
  workspace_agents
SET
  first_connected_at = $2,
  last_connected_at = $3,
  disconnected_at = $4
WHERE
  id = $1;

-- name: UpdateWorkspaceBuildByID :exec
UPDATE
  workspace_builds
SET
  updated_at = $2,
  after_id = $3,
  provisioner_state = $4
WHERE
  id = $1;
