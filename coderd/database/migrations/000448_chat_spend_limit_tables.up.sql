-- Add new tables:
CREATE TABLE chat_user_spend_limits (
	user_id UUID PRIMARY KEY REFERENCES users(id),
	spend_limit_micros BIGINT
);
ALTER TABLE chat_user_spend_limits
	ADD CONSTRAINT chat_user_spend_limits_check CHECK (((spend_limit_micros IS NULL) OR (spend_limit_micros > 0)));
COMMENT ON TABLE chat_user_spend_limits IS 'Experimental (agents): Stores per-user spend limits for chat usage. A NULL value indicates no limit.';

-- groups are hard-deleted so we need ON DELETE CASCADE
CREATE TABLE chat_group_spend_limits (
	group_id UUID PRIMARY KEY REFERENCES groups(id) ON DELETE CASCADE,
	spend_limit_micros BIGINT
);
ALTER TABLE chat_group_spend_limits
	ADD CONSTRAINT chat_group_spend_limits_check CHECK (((spend_limit_micros IS NULL) OR (spend_limit_micros > 0)));
COMMENT ON TABLE chat_group_spend_limits IS 'Experimental (agents): Stores per-group spend limits for chat usage. A NULL value indicates no limit.';

-- Migrate existing data
INSERT INTO chat_user_spend_limits (user_id, spend_limit_micros)
	SELECT u.id, u.chat_spend_limit_micros FROM users u;

INSERT INTO chat_group_spend_limits (group_id, spend_limit_micros)
	SELECT g.id, g.chat_spend_limit_micros FROM groups g;

-- Drop old columns
ALTER TABLE users DROP COLUMN chat_spend_limit_micros;
ALTER TABLE groups DROP COLUMN chat_spend_limit_micros;
