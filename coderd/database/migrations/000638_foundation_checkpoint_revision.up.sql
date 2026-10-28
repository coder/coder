ALTER TABLE template_resource_states
    ADD COLUMN checkpoint_revision_id uuid,
    ADD FOREIGN KEY (checkpoint_revision_id, foundation_id, organization_id)
        REFERENCES template_resource_revisions(id, foundation_id, organization_id);

-- Only a published successful apply establishes known checkpoint provenance.
-- Existing quarantines remain quarantined; their saved state may be uncertain.
UPDATE template_resource_states
SET checkpoint_revision_id = applied_revision_id
WHERE status != 'unknown';

COMMENT ON COLUMN template_resource_states.checkpoint_revision_id IS
    'Revision that produced the authoritative state checkpoint, independent of the last published outputs.';
