-- Fix the unique index in `custom_roles` to allow the same role name
-- in different organizations. The original index only covered name,
-- but names don't have to be unique across different organizations.
--
-- Note: after fixing it, we end up with an almost-replica of the
-- existing `custom_roles_unique_key` constraint. That's unfortunate,
-- but since we can't define a constraint on an expression (e.g. lower()),
-- we'll have to keep both of them.
DROP INDEX IF EXISTS idx_custom_roles_name_lower;

-- Use `COALESCE` to handle `NULL` organization_id. Site-wide custom
-- roles are currently not used, but that can change in the future and
-- this will become necessary. And there are no performance implications.
--
-- Note: Using `NULLS NOT DISTINCT` instead of `COALESCE` here would
-- limit us to PG15+.

-- Paranoia check.
UPDATE custom_roles SET organization_id = NULL WHERE organization_id = '00000000-0000-0000-0000-000000000000';

ALTER TABLE custom_roles
    ADD CONSTRAINT organization_id_not_zero
    CHECK (organization_id <> '00000000-0000-0000-0000-000000000000'::uuid);

CREATE UNIQUE INDEX idx_custom_roles_name_lower_organization_id ON custom_roles USING btree (
    LOWER(name),
    COALESCE(organization_id, '00000000-0000-0000-0000-000000000000'::uuid)
);
