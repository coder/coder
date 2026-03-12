ALTER TABLE chat_messages ALTER COLUMN total_cost_micros TYPE BIGINT USING total_cost_micros::bigint;
