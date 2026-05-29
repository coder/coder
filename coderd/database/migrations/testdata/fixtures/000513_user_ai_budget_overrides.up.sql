-- Seed a group_members row so the override below references a real
-- membership.
INSERT INTO group_members (
    user_id,
    group_id
) VALUES
    ('30095c71-380b-457a-8995-97b8ee6e5307', 'bb640d07-ca8a-4869-b6bc-ae61ebb2fda1')
ON CONFLICT DO NOTHING;

INSERT INTO user_ai_budget_overrides (
    user_id,
    group_id,
    spend_limit_micros
) VALUES
    ('30095c71-380b-457a-8995-97b8ee6e5307', 'bb640d07-ca8a-4869-b6bc-ae61ebb2fda1', 500000000);
