INSERT INTO agent_runtime_hourly_usage (bucket_start, organization_id, group_id, user_id, runtime_ms)
VALUES ('2026-10-01 12:00:00+00', 'bb640d07-ca8a-4869-b6bc-ae61ebb2fda1', 'bb640d07-ca8a-4869-b6bc-ae61ebb2fda1', '30095c71-380b-457a-8995-97b8ee6e5307', 600000);

-- The migration deleted the 000615 fixture's Everyone allotment, so keep a
-- group allotment for the fixture table check.
INSERT INTO groups (id, name, organization_id)
VALUES ('6a1f0c3e-2b4d-4e8f-9a7b-5c3d2e1f0a9b', 'agent-hours-fixture', 'bb640d07-ca8a-4869-b6bc-ae61ebb2fda1');

INSERT INTO agent_hours_group_allotments (group_id, allotment_bps)
VALUES ('6a1f0c3e-2b4d-4e8f-9a7b-5c3d2e1f0a9b', 5000);
