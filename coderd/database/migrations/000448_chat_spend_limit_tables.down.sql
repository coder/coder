-- Restore old columns
ALTER TABLE users ADD COLUMN chat_spend_limit_micros BIGINT;
ALTER TABLE users ADD CONSTRAINT users_chat_spend_limit_micros_check CHECK (((chat_spend_limit_micros IS NULL) OR (chat_spend_limit_micros > 0)));
ALTER TABLE groups ADD COLUMN chat_spend_limit_micros BIGINT;
ALTER TABLE groups ADD CONSTRAINT groups_chat_spend_limit_micros_check CHECK (((chat_spend_limit_micros IS NULL) OR (chat_spend_limit_micros > 0)));

-- Migrate data from chat_user_spend_limits to users
UPDATE users u
SET chat_spend_limit_micros = c.spend_limit_micros
FROM chat_user_spend_limits c
WHERE u.id = c.user_id;

-- Migrate data from chat_group_spend_limits to groups
UPDATE groups g
SET chat_spend_limit_micros = c.spend_limit_micros
FROM chat_group_spend_limits c
WHERE g.id = c.group_id;

-- Drop chat spend limit tables
DROP TABLE chat_user_spend_limits;
DROP TABLE chat_group_spend_limits;
