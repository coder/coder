-- Remove Task 'completed' transition template notification
DELETE FROM notification_templates WHERE id = '8c5a4d12-9f7e-4b3a-a1c8-6e4f2d9b5a7c';

-- Remove Task 'failed' transition template notification
DELETE FROM notification_templates WHERE id = '3b7e8f1a-4c2d-49a6-b5e9-7f3a1c8d6b4e';
