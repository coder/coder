-- Fixture for migration 000417_workspace_acl_object_constraint.
-- Inserts a workspace with 'null'::json ACLs to ensure the migration
-- correctly normalizes such values.

INSERT INTO workspaces (
    id,
    created_at,
    updated_at,
    owner_id,
    organization_id,
    template_id,
    deleted,
    name,
    last_used_at,
    automatic_updates,
    favorite,
    group_acl,
    user_acl
)
VALUES (
    '6f6fdbee-4c18-4a5c-8a8d-9b811c9f0a28',
    '2024-02-10 00:00:00+00',
    '2024-02-10 00:00:00+00',
    '30095c71-380b-457a-8995-97b8ee6e5307',
    'bb640d07-ca8a-4869-b6bc-ae61ebb2fda1',
    '4cc1f466-f326-477e-8762-9d0c6781fc56',
    false,
    'acl-null-workspace',
    '0001-01-01 00:00:00+00',
    'never',
    false,
    'null'::jsonb,
    'null'::jsonb
)
ON CONFLICT DO NOTHING;
