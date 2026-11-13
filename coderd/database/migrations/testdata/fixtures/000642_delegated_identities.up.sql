-- A service account from the fixture of migration 000433 becomes a delegated
-- identity under the first user.
INSERT INTO delegated_identities (
	id,
	organization_id,
	parent_id,
	root_user_id,
	created_by,
	name,
	scopes,
	created_at,
	updated_at
)
VALUES (
	'b2ce097d-2287-4d64-a550-ed821969545d',
	'bb640d07-ca8a-4869-b6bc-ae61ebb2fda1',
	'30095c71-380b-457a-8995-97b8ee6e5307',
	'30095c71-380b-457a-8995-97b8ee6e5307',
	'30095c71-380b-457a-8995-97b8ee6e5307',
	'fixture-service-account-1',
	'{workspace:read}',
	'2024-11-02 13:10:00.000000+02',
	'2024-11-02 13:10:00.000000+02'
);
