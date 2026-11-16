INSERT INTO chat_projects (id, organization_id, owner_id, name)
VALUES ('59400000-0000-4000-8000-000000000002', 'bb640d07-ca8a-4869-b6bc-ae61ebb2fda1', '0ed9befc-4911-4ccf-a8e2-559bf72daa94', 'Fixture Unpinned Project');

-- One pin and one tombstone.
INSERT INTO chat_project_pins (user_id, project_id, pin_order, source, unpinned_at)
VALUES
    ('30095c71-380b-457a-8995-97b8ee6e5307', '59400000-0000-4000-8000-000000000001', 1, 'user', NULL),
    ('0ed9befc-4911-4ccf-a8e2-559bf72daa94', '59400000-0000-4000-8000-000000000002', 0, 'default', '2026-01-01 00:00:00+00');
