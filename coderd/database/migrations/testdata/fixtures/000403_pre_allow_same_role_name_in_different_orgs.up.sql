-- Fixture for migration 000404_allow_same_role_name_in_different_orgs.
-- Inserts a custom role with an all-zero organization_id to ensure the
-- migration doesn't choke on such rows.
INSERT INTO custom_roles (name, display_name, organization_id)
VALUES (
	'custom-role-zero-org-id',
	'Custom Role (Zero Org ID)',
	'00000000-0000-0000-0000-000000000000'::uuid
)
ON CONFLICT DO NOTHING;
