CREATE TYPE prebuild_status AS ENUM (
  'healthy',          -- Prebuilds are working as expected; this is the default, healthy state.
  'hard_limited',     -- Prebuilds have failed repeatedly and hit the configured hard failure limit; won't be retried anymore.
  'validation_failed' -- Prebuilds failed due to a non-retryable validation error (e.g. template misconfiguration); won't be retried.
);

ALTER TABLE template_version_presets ADD COLUMN prebuild_status prebuild_status NOT NULL DEFAULT 'healthy'::prebuild_status;
