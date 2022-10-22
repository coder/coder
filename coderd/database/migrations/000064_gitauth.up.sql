CREATE TABLE IF NOT EXISTS git_auth_links (
  provider_id text NOT NULL,
  user_id uuid NOT NULL,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  oauth_access_token text NOT NULL,
  oauth_refresh_token text NOT NULL,
  oauth_expiry timestamptz NOT NULL,
  UNIQUE(provider_id, user_id)
);
