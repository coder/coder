ALTER TABLE chats ADD COLUMN spend_limit_micros BIGINT NULL;
ALTER TABLE chats ADD CONSTRAINT chats_spend_limit_micros_positive CHECK (spend_limit_micros > 0);
