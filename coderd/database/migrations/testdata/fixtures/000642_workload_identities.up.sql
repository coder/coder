INSERT INTO workload_identities (
	id,
	created_at,
	organization_id,
	sponsor_id,
	workspace_id,
	name,
	scopes,
	workspace_agent_id,
	attestation_subject,
	bound_at
)
VALUES (
	'8c2f4b1e-6d3a-4e7f-9b0c-1a5d7e9f2c4b',
	'2024-11-02 13:10:00.000000+02',
	'bb640d07-ca8a-4869-b6bc-ae61ebb2fda1',
	'30095c71-380b-457a-8995-97b8ee6e5307',
	'3a9a1feb-e89d-457c-9d53-ac751b198ebe',
	'workload-1',
	'{workspace:read}',
	'45e89705-e09d-4850-bcec-f9a937f5d78d',
	'k8s-pod:3f1c0a52-7c9e-4d0b-8a57-0d6f1c2b3a4e',
	'2024-11-02 13:10:05.000000+02'
);
