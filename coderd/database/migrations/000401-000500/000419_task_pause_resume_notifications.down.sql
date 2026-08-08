-- Remove Task 'paused' transition template notification
DELETE FROM notification_templates WHERE id = '2a74f3d3-ab09-4123-a4a5-ca238f4f65a1';
-- Remove Task 'resumed' transition template notification
DELETE FROM notification_templates WHERE id = '843ee9c3-a8fb-4846-afa9-977bec578649';
