-- Remove Task 'working' transition template notification
DELETE FROM notification_templates WHERE id = 'bd4b7168-d05e-4e19-ad0f-3593b77aa90f';
-- Remove Task 'idle' transition template notification
DELETE FROM notification_templates WHERE id = 'd4a6271c-cced-4ed0-84ad-afd02a9c7799';
