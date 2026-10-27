-- Share the fixture artifact with the organization's Everyone group.
UPDATE chat_artifacts
SET group_acl = '{"bb640d07-ca8a-4869-b6bc-ae61ebb2fda1": {"permissions": ["read"]}}'::jsonb
WHERE id = '59600000-0000-4000-8000-000000000001';
