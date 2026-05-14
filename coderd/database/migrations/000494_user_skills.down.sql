-- Enum additions to resource_type and api_key_scope are intentionally not
-- reverted because Postgres cannot drop enum values safely.
DROP TRIGGER IF EXISTS trigger_user_skills_per_user_limit ON user_skills;
DROP FUNCTION IF EXISTS enforce_user_skills_per_user_limit();
DROP TABLE user_skills;
