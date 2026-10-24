ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'github_runner_pool:*';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'github_runner_pool:create';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'github_runner_pool:delete';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'github_runner_pool:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'github_runner_pool:update';
