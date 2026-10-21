-- name: InsertChatFile :one
-- A NULL purpose stores a prompt file.
INSERT INTO chat_files (owner_id, organization_id, name, mimetype, data, purpose)
VALUES (
	@owner_id::uuid,
	@organization_id::uuid,
	@name::text,
	@mimetype::text,
	@data::bytea,
	COALESCE(sqlc.narg('purpose')::chat_file_purpose, 'prompt'::chat_file_purpose)
)
RETURNING id, owner_id, organization_id, created_at, name, mimetype, purpose;

-- name: GetChatFileByID :one
SELECT * FROM chat_files WHERE id = @id::uuid;

-- name: GetChatFilesByIDs :many
SELECT * FROM chat_files WHERE id = ANY(@ids::uuid[]);

-- name: GetChatFileDataPrefixesByIDs :many
-- GetChatFileDataPrefixesByIDs returns a bounded prefix of each
-- file's content, keeping full blobs out of server memory. Owner and
-- organization columns support row-level authorization.
SELECT id, owner_id, organization_id, substr(data, 1, @prefix_bytes::int) AS data_prefix
FROM chat_files
WHERE id = ANY(@ids::uuid[]);

-- name: GetChatFileMetadataByChatID :many
-- GetChatFileMetadataByChatID returns lightweight file metadata for
-- all files linked to a chat. The data column is excluded to avoid
-- loading file content.
SELECT cf.id, cf.owner_id, cf.organization_id, cf.name, cf.mimetype, cf.created_at,
	COALESCE(cf.workspace_delivered_size_bytes, octet_length(cf.data))::bigint AS size_bytes,
	cf.purpose,
	cf.workspace_delivery_attempts,
	cf.workspace_delivery_next_attempt_at,
	cf.workspace_delivery_error,
	cf.workspace_delivered_at,
	cf.workspace_delivered_path,
	cf.workspace_delivered_workspace_id
FROM chat_files cf
JOIN chat_file_links cfl ON cfl.file_id = cf.id
WHERE cfl.chat_id = @chat_id::uuid
ORDER BY cf.created_at ASC;

-- name: GetChatFilesMetadataByIDs :many
-- GetChatFilesMetadataByIDs returns file metadata without loading file
-- content. Owner and organization columns support row-level authorization.
-- Delivered workspace files report the size they had before delivery.
SELECT id, owner_id, organization_id, created_at, name, mimetype,
	COALESCE(workspace_delivered_size_bytes, octet_length(data))::bigint AS size_bytes,
	purpose,
	workspace_delivered_at
FROM chat_files
WHERE id = ANY(@ids::uuid[]);

-- name: AcquireChatWorkspaceFileDeliveries :many
-- AcquireChatWorkspaceFileDeliveries claims due workspace files whose
-- chat's workspace finished starting: the workspace's latest build is a
-- start build and the agent a delivery would use reports ready,
-- start_error or start_timeout. That agent is picked like
-- agentselect.FindChatAgent over the latest build's agents, after the
-- chat's bound agent: the bound agent when it is in the latest build,
-- else the single root agent whose name ends with chat_agent_suffix,
-- else the first root agent by display order, then name, then ID. When
-- several root agents match the suffix, the first is checked here and
-- the caller's selection reports the ambiguity. A file is due when its
-- next attempt time has passed and attempts remain. It is also due, with
-- a fresh attempt budget, when that agent's ready_at differs from the one
-- recorded at the file's last claim, unless an unexpired lease without an
-- error holds it: every agent startup retries failed and parked files
-- once, with no startup event needed. A claim records the agent's
-- ready_at, counts an attempt, clears the last error and leases the file
-- until
-- lease_expires_at plus the time the content takes to upload at
-- min_upload_bytes_per_second, so the lease of a large file covers its
-- upload. The returned lease_expires_at identifies the claim:
-- the outcome queries take it back so a worker whose lease expired and
-- was claimed again cannot record an outcome. File content is not
-- returned so a batch claim does not load blobs.
WITH candidates AS (
	SELECT
		cf.id,
		c.id AS chat_id,
		c.workspace_id,
		selected_agent.ready_at AS agent_ready_at,
		(selected_agent.ready_at IS DISTINCT FROM cf.workspace_delivery_agent_ready_at) AS agent_restarted
	FROM chat_files cf
	JOIN chat_file_links cfl ON cfl.file_id = cf.id
	JOIN chats c ON c.id = cfl.chat_id
	JOIN LATERAL (
		SELECT wb.job_id, wb.transition
		FROM workspace_builds wb
		WHERE wb.workspace_id = c.workspace_id
		ORDER BY wb.build_number DESC
		LIMIT 1
	) latest_build ON TRUE
	JOIN LATERAL (
		SELECT wa.lifecycle_state, wa.ready_at
		FROM workspace_agents wa
		JOIN workspace_resources wr ON wr.id = wa.resource_id
		WHERE wr.job_id = latest_build.job_id
			AND wa.deleted = FALSE
			AND (wa.parent_id IS NULL OR wa.id = c.agent_id)
		ORDER BY
			(wa.id = c.agent_id) IS TRUE DESC,
			(right(lower(wa.name), char_length(@chat_agent_suffix::text)) = lower(@chat_agent_suffix::text)) DESC,
			wa.display_order ASC,
			lower(wa.name) COLLATE "C" ASC,
			wa.name COLLATE "C" ASC,
			wa.id ASC
		LIMIT 1
	) selected_agent ON TRUE
	WHERE cf.purpose = 'workspace'::chat_file_purpose
		AND cf.workspace_delivered_at IS NULL
		AND (
			(
				(cf.workspace_delivery_next_attempt_at IS NULL OR cf.workspace_delivery_next_attempt_at <= @now::timestamptz)
				AND cf.workspace_delivery_attempts < @max_attempts::int
			)
			OR (
				selected_agent.ready_at IS DISTINCT FROM cf.workspace_delivery_agent_ready_at
				AND (
					cf.workspace_delivery_error IS NOT NULL
					OR cf.workspace_delivery_next_attempt_at IS NULL
					OR cf.workspace_delivery_next_attempt_at <= @now::timestamptz
				)
			)
		)
		AND c.archived = FALSE
		AND (sqlc.narg('chat_id')::uuid IS NULL OR c.id = sqlc.narg('chat_id')::uuid)
		AND latest_build.transition = 'start'::workspace_transition
		AND selected_agent.lifecycle_state IN (
			'ready'::workspace_agent_lifecycle_state,
			'start_error'::workspace_agent_lifecycle_state,
			'start_timeout'::workspace_agent_lifecycle_state
		)
	ORDER BY cf.workspace_delivery_next_attempt_at ASC NULLS FIRST, cf.created_at ASC, cf.id ASC
	LIMIT @limit_val::int
	FOR UPDATE OF cf SKIP LOCKED
)
UPDATE chat_files cf
SET
	workspace_delivery_attempts = CASE WHEN candidates.agent_restarted THEN 1 ELSE cf.workspace_delivery_attempts + 1 END,
	workspace_delivery_agent_ready_at = candidates.agent_ready_at,
	workspace_delivery_next_attempt_at = make_interval(secs => octet_length(cf.data) / @min_upload_bytes_per_second::float8) + @lease_expires_at::timestamptz,
	workspace_delivery_error = NULL
FROM candidates
WHERE cf.id = candidates.id
RETURNING
	cf.id,
	cf.owner_id,
	cf.organization_id,
	cf.name,
	cf.mimetype,
	octet_length(cf.data)::bigint AS size_bytes,
	cf.workspace_delivery_attempts,
	cf.workspace_delivery_next_attempt_at::timestamptz AS lease_expires_at,
	candidates.chat_id,
	candidates.workspace_id;

-- name: CompleteChatWorkspaceFileDelivery :execrows
-- CompleteChatWorkspaceFileDelivery records the receipt and drops the
-- file content. Metadata stays so messages that reference the file keep
-- validating. It applies only under the claim that lease_expires_at
-- identifies and while the file's chat is still bound to the delivered
-- workspace, so a chat that moved to another workspace during the upload
-- keeps the file for the new workspace.
UPDATE chat_files
SET
	data = ''::bytea,
	workspace_delivered_size_bytes = octet_length(data),
	workspace_delivered_at = @delivered_at::timestamptz,
	workspace_delivered_path = @delivered_path::text,
	workspace_delivered_workspace_id = @delivered_workspace_id::uuid,
	workspace_delivery_error = NULL,
	workspace_delivery_next_attempt_at = NULL
WHERE id = @id::uuid
	AND purpose = 'workspace'::chat_file_purpose
	AND workspace_delivered_at IS NULL
	AND workspace_delivery_next_attempt_at = @lease_expires_at::timestamptz
	AND EXISTS (
		SELECT 1
		FROM chat_file_links cfl
		JOIN chats c ON c.id = cfl.chat_id
		WHERE cfl.file_id = chat_files.id
			AND c.workspace_id = @delivered_workspace_id::uuid
	);

-- name: FailChatWorkspaceFileDelivery :execrows
-- FailChatWorkspaceFileDelivery records a failed attempt. The mode
-- decides the claimed attempt: 'retry' keeps it counted, 'refund' gives
-- it back for failures that never reached the agent, and 'park' sets the
-- attempts to max_attempts so the file waits for an explicit retry. Any
-- other mode fails on the NOT NULL attempts column. It applies only under
-- the claim that lease_expires_at identifies.
UPDATE chat_files
SET
	workspace_delivery_error = @error::text,
	workspace_delivery_next_attempt_at = @next_attempt_at::timestamptz,
	workspace_delivery_attempts = CASE @mode::text
		WHEN 'retry' THEN workspace_delivery_attempts
		WHEN 'refund' THEN GREATEST(workspace_delivery_attempts - 1, 0)
		WHEN 'park' THEN GREATEST(workspace_delivery_attempts, @max_attempts::int)
		ELSE NULL
	END
WHERE id = @id::uuid
	AND purpose = 'workspace'::chat_file_purpose
	AND workspace_delivered_at IS NULL
	AND workspace_delivery_next_attempt_at = @lease_expires_at::timestamptz;

-- name: RetryChatWorkspaceFileDeliveries :execrows
-- RetryChatWorkspaceFileDeliveries makes a chat's undelivered workspace
-- files due again with a fresh attempt budget. Only a file in flight
-- under a lease that has not expired at now is left alone. An expired
-- lease without an error means an attempt never recorded its outcome,
-- for example because its replica stopped, and may have used the last
-- attempt.
UPDATE chat_files cf
SET
	workspace_delivery_attempts = 0,
	workspace_delivery_next_attempt_at = NULL,
	workspace_delivery_error = NULL
FROM chat_file_links cfl
WHERE cfl.file_id = cf.id
	AND cfl.chat_id = @chat_id::uuid
	AND cf.purpose = 'workspace'::chat_file_purpose
	AND cf.workspace_delivered_at IS NULL
	AND (
		cf.workspace_delivery_error IS NOT NULL
		OR cf.workspace_delivery_next_attempt_at IS NULL
		OR cf.workspace_delivery_next_attempt_at <= @now::timestamptz
	);

-- name: GetOldUnlinkedChatFileIDs :many
-- Locks candidate rows against foreign-key inserts for the transaction.
SELECT cf.id
FROM chat_files cf
WHERE cf.created_at < @before_time::timestamptz
  AND NOT EXISTS (
    SELECT 1 FROM chat_file_links cfl WHERE cfl.file_id = cf.id
  )
ORDER BY cf.created_at ASC
LIMIT @limit_count
FOR UPDATE OF cf SKIP LOCKED;

-- name: DeleteUnlinkedChatFilesByIDs :execrows
DELETE FROM chat_files cf
WHERE cf.id = ANY(@ids::uuid[])
  AND cf.created_at < @before_time::timestamptz
  AND NOT EXISTS (
    SELECT 1 FROM chat_file_links cfl WHERE cfl.file_id = cf.id
  );
