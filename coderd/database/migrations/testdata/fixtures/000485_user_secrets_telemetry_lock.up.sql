-- Smoke fixture: a single user_secrets_summary lock for a fixed period.
INSERT INTO telemetry_locks (event_type, period_ending_at)
VALUES ('user_secrets_summary', '2026-01-01 00:00:00+00');
