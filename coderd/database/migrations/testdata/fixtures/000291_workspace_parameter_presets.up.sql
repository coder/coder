-- Organization
INSERT INTO organizations (id, name, description, display_name, created_at, updated_at)
VALUES ('d3fd38d2-ffc3-4ec2-8cfc-9c8dab6d9a74', 'Test Org', 'Test Organization', 'Test Org', now(), now());

-- User
INSERT INTO users (id, email, username, created_at, updated_at, status, rbac_roles, login_type, hashed_password)
VALUES ('1f573504-f7a0-4498-8b81-2e1939f3c4a2', 'test@coder.com', 'testuser', now(), now(), 'active', '{}', 'password', 'password');

-- Provisioner Job
INSERT INTO provisioner_jobs (id, organization_id, created_at, updated_at, initiator_id, provisioner, storage_method, type, input, file_id)
VALUES ('50ebe702-82e5-4053-859d-c24a3b742b57', 'd3fd38d2-ffc3-4ec2-8cfc-9c8dab6d9a74', now(), now(), '1f573504-f7a0-4498-8b81-2e1939f3c4a2', 'echo', 'file', 'template_version_import', '{}', '00000000-82e5-4053-859d-c24a3b742b57');

-- Template Version
INSERT INTO template_versions (id, organization_id, created_by, created_at, updated_at, name, job_id, readme, message)
VALUES ('f1276e15-01cd-406d-8ea5-64f113a79601', 'd3fd38d2-ffc3-4ec2-8cfc-9c8dab6d9a74', '1f573504-f7a0-4498-8b81-2e1939f3c4a2', now(), now(), 'test-version', '50ebe702-82e5-4053-859d-c24a3b742b57', '', '');

-- Template
INSERT INTO templates (id, created_by, organization_id, created_at, updated_at, deleted, name, provisioner, active_version_id, description)
VALUES ('0bd0713b-176a-4864-a58b-546a1b021025', '1f573504-f7a0-4498-8b81-2e1939f3c4a2', 'd3fd38d2-ffc3-4ec2-8cfc-9c8dab6d9a74', now(), now(), false, 'test-template', 'terraform', 'f1276e15-01cd-406d-8ea5-64f113a79601', 'Test template');

UPDATE templates SET active_version_id = 'f1276e15-01cd-406d-8ea5-64f113a79601' WHERE id = '0bd0713b-176a-4864-a58b-546a1b021025';
-- Workspace
INSERT INTO workspaces (id, organization_id, owner_id, template_id, created_at, updated_at, name, deleted, automatic_updates)
VALUES ('8cb0b7c4-47b5-4bfc-ad92-88ccc61f3c12', 'd3fd38d2-ffc3-4ec2-8cfc-9c8dab6d9a74', '1f573504-f7a0-4498-8b81-2e1939f3c4a2', '0bd0713b-176a-4864-a58b-546a1b021025', now(), now(), 'test-workspace', false, 'never');

-- Workspace Build
INSERT INTO workspace_builds (id, workspace_id, template_version_id, initiator_id, job_id, created_at, updated_at, transition, reason, build_number)
VALUES ('83b28647-743c-4649-b226-f2be697ca06c', '8cb0b7c4-47b5-4bfc-ad92-88ccc61f3c12', 'f1276e15-01cd-406d-8ea5-64f113a79601', '1f573504-f7a0-4498-8b81-2e1939f3c4a2', '50ebe702-82e5-4053-859d-c24a3b742b57', now(), now(), 'start', 'initiator', 45);

-- Template Version Presets
INSERT INTO template_version_presets (id, template_version_id, name, created_at)
VALUES
    ('575a0fbb-cc3e-4709-ae9f-d1a3f365909c', 'f1276e15-01cd-406d-8ea5-64f113a79601', 'test', now()),
    ('2c76596d-436d-42eb-a38c-8d5a70497030', 'f1276e15-01cd-406d-8ea5-64f113a79601', 'test', now());
