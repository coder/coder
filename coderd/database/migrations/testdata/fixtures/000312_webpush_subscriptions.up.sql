-- VAPID keys lited from coderd/notifications_test.go.
INSERT INTO webpush_subscriptions (id, user_id, created_at, endpoint, endpoint_p256dh_key, endpoint_auth_key) VALUES (gen_random_uuid(), (SELECT id FROM users LIMIT 1), NOW(), 'https://example.com', 'BNNL5ZaTfK81qhXOx23+wewhigUeFb632jN6LvRWCFH1ubQr77FE/9qV1FuojuRmHP42zmf34rXgW80OvUVDgTk=', 'zqbxT6JKstKSY9JKibZLSQ==');
