INSERT INTO workspace_execution_sessions (
 id, organization_id, owner_id, actor_id, request_id, input_digest,
 created_at, updated_at, state, lease_expires_at, declarations
)
SELECT 'f0000000-0000-4000-8000-000000000604', o.id, u.id, u.id,
 'f1000000-0000-4000-8000-000000000604', sha256('acquisition'::bytea),
 NOW(), NOW(), 'retained', NOW(), '{"result_paths":["/result.bin"]}'::jsonb
FROM (SELECT id FROM organizations ORDER BY id LIMIT 1) o
CROSS JOIN (SELECT id FROM users ORDER BY id LIMIT 1) u;

INSERT INTO workspace_execution_receipts (
 id, session_id, organization_id, owner_id, actor_id, request_id,
 input_digest, workspace_id, workspace_owner_id, agent_id, agent_instance_id,
 process_id, admission_revision, created_at, updated_at, state, exit_code
)
SELECT 'f0000000-0000-4000-8000-000000000605', id, organization_id, owner_id,
 actor_id, 'f1000000-0000-4000-8000-000000000605', sha256('command'::bytea),
 'f2000000-0000-4000-8000-000000000605', owner_id,
 'f3000000-0000-4000-8000-000000000605', 'f4000000-0000-4000-8000-000000000605',
 'f5000000-0000-4000-8000-000000000605', revision, NOW(), NOW(), 'completed', 0
FROM workspace_execution_sessions
WHERE id = 'f0000000-0000-4000-8000-000000000604';

INSERT INTO workspace_execution_artifacts (
 id, organization_id, owner_id, session_id, preservation_revision,
 source_path, name, mimetype, size_bytes, sha256, data, created_at
)
SELECT 'f0000000-0000-4000-8000-000000000606', organization_id, owner_id,
 id, revision, '/result.bin', 'result.bin', 'application/octet-stream', 3,
 sha256(decode('00ff01', 'hex')), decode('00ff01', 'hex'), NOW()
FROM workspace_execution_sessions
WHERE id = 'f0000000-0000-4000-8000-000000000604';

INSERT INTO chat_submissions (
 id, organization_id, actor_id, owner_id, request_id, input_digest,
 kind, chat_id, state, error
)
SELECT 'f0000000-0000-4000-8000-000000000607', organization_id, actor_id,
 owner_id, 'f1000000-0000-4000-8000-000000000607', sha256('chat'::bytea),
 'create', 'f2000000-0000-4000-8000-000000000607', 'rejected', 'controlled rejection'
FROM workspace_execution_sessions
WHERE id = 'f0000000-0000-4000-8000-000000000604';

UPDATE workspace_execution_sessions SET state = 'preserved'
WHERE id = 'f0000000-0000-4000-8000-000000000604';

UPDATE workspace_execution_sessions SET recovery_artifact_expires_at = NOW() + interval '1 day'
WHERE id = 'f0000000-0000-4000-8000-000000000604';
