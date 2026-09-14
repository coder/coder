-- Make webpush subscriptions idempotent on (user_id, endpoint).
--
-- Without a unique constraint, a re-subscribe with the same endpoint
-- (which Apple Web Push and other push services do when keys rotate
-- without endpoint deactivation, including after a PWA reinstall on
-- iOS) inserts a duplicate row carrying the new keys. Dispatch then
-- delivers to both endpoints; the device cannot decrypt the old one
-- and silently drops it.
--
-- Dedupe existing rows before adding the index. Keep the freshest row
-- per (user_id, endpoint) since it most likely matches the device's
-- current p256dh / auth keys. The duplicates being deleted here are
-- by definition stale.
DELETE FROM webpush_subscriptions a
USING webpush_subscriptions b
WHERE a.user_id = b.user_id
  AND a.endpoint = b.endpoint
  AND (a.created_at, a.id) < (b.created_at, b.id);

CREATE UNIQUE INDEX webpush_subscriptions_user_id_endpoint_idx
    ON webpush_subscriptions (user_id, endpoint);
