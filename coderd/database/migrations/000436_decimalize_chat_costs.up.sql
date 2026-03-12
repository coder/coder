ALTER TABLE chat_messages ALTER COLUMN total_cost_micros TYPE NUMERIC USING total_cost_micros::numeric;
