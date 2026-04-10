-- Step 1: Add nullable column with FK.
ALTER TABLE chats
    ADD COLUMN organization_id UUID REFERENCES organizations(id) ON DELETE CASCADE;

-- Step 2: Backfill from workspace org (primary path). Fall back to
-- user's oldest org membership, then default org for rows where
-- workspace_id was NULLed out by ON DELETE SET NULL or never set.
UPDATE chats c
SET organization_id = COALESCE(
    (SELECT w.organization_id FROM workspaces w WHERE w.id = c.workspace_id),
    (SELECT om.organization_id FROM organization_members om
     WHERE om.user_id = c.owner_id ORDER BY om.created_at ASC LIMIT 1),
    (SELECT id FROM organizations WHERE is_default = true LIMIT 1)
);

-- Step 3: Enforce NOT NULL going forward.
ALTER TABLE chats ALTER COLUMN organization_id SET NOT NULL;

-- Step 4: Index for efficient lookups by organization.
CREATE INDEX idx_chats_organization_id ON chats (organization_id);
