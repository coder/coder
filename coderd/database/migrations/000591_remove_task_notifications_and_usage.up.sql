-- Delete task notification templates and preferences. The delete cascades
-- to inbox_notifications and notification_messages: historical task
-- notifications are intentionally removed with the feature.
DELETE FROM notification_templates
WHERE id IN (
    'bd4b7168-d05e-4e19-ad0f-3593b77aa90f', -- Task Working
    'd4a6271c-cced-4ed0-84ad-afd02a9c7799', -- Task Idle
    '8c5a4d12-9f7e-4b3a-a1c8-6e4f2d9b5a7c', -- Task Completed
    '3b7e8f1a-4c2d-49a6-b5e9-7f3a1c8d6b4e', -- Task Failed
    '2a74f3d3-ab09-4123-a4a5-ca238f4f65a1', -- Task Paused
    '843ee9c3-a8fb-4846-afa9-977bec578649'  -- Task Resumed
);

DELETE FROM user_configs
WHERE key = 'preference_task_notification_alert_dismissed';

-- Remove the task AI seat usage reason. Seats consumed by tasks stay
-- consumed; their last event type is remapped to aibridge.
UPDATE ai_seat_state SET last_event_type = 'aibridge' WHERE last_event_type::text = 'task';

ALTER TYPE ai_seat_usage_reason RENAME TO ai_seat_usage_reason_old;

CREATE TYPE ai_seat_usage_reason AS ENUM (
    'aibridge'
);

ALTER TABLE ai_seat_state ALTER COLUMN last_event_type TYPE ai_seat_usage_reason USING (last_event_type::text::ai_seat_usage_reason);

DROP TYPE ai_seat_usage_reason_old;
