CREATE TABLE IF NOT EXISTS git_provider_links (
  user_id uuid NOT NULL,
  url text NOT NULL,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  oauth_access_token text NOT NULL,
  oauth_refresh_token text NOT NULL,
  oauth_expiry text NOT NULL
);
