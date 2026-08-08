-- webpush_subscriptions is a table that stores push notification
-- subscriptions for users. These are acquired via the Push API in the browser.
CREATE TABLE IF NOT EXISTS webpush_subscriptions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users ON DELETE CASCADE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    -- endpoint is called by coderd to send a push notification to the user.
    endpoint TEXT NOT NULL,
    -- endpoint_p256dh_key is the public key for the endpoint.
    endpoint_p256dh_key TEXT NOT NULL,
    -- endpoint_auth_key is the authentication key for the endpoint.
    endpoint_auth_key TEXT NOT NULL
);
