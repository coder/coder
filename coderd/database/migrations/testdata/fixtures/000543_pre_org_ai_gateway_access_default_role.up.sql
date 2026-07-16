-- Organizations in their pre-000544 state, exercising both branches of the
-- data migration: one with the legacy default member roles (the role gets
-- appended) and one that already carries organization-ai-gateway-access
-- (the idempotency guard must not append a duplicate).
INSERT INTO organizations (id, name, description, created_at, updated_at, is_default, display_name, icon, default_org_member_roles)
VALUES (
	'93e5e735-e755-46f0-9c7c-b4185cd34d9e',
	'legacy_default_roles_org',
	'org with the pre-000544 default member roles',
	'2026-07-16 00:00:00 +00:00',
	'2026-07-16 00:00:00 +00:00',
	false,
	'legacy_default_roles_org',
	'',
	ARRAY['organization-workspace-access']::text[]
);

INSERT INTO organizations (id, name, description, created_at, updated_at, is_default, display_name, icon, default_org_member_roles)
VALUES (
	'b3f1fa49-2ea5-4a39-b78a-fdbe71879dbe',
	'ai_gateway_role_org',
	'org that already has organization-ai-gateway-access',
	'2026-07-16 00:00:00 +00:00',
	'2026-07-16 00:00:00 +00:00',
	false,
	'ai_gateway_role_org',
	'',
	ARRAY['organization-workspace-access', 'organization-ai-gateway-access']::text[]
);
