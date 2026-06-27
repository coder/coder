-- Add 'claude-platform-aws' to ai_provider_type. The aibridge runtime supports
-- Claude Platform for AWS via the Anthropic client with a Claude Platform
-- discriminator in Settings (SigV4 service aws-external-anthropic, regional host
-- aws-external-anthropic.<region>.api.aws, anthropic-workspace-id header). The
-- enum just needs the discriminator so DB-driven providers can carry it. Mirrors
-- the precedent in 000506_ai_provider_type_copilot_value.up.sql.
ALTER TYPE ai_provider_type ADD VALUE IF NOT EXISTS 'claude-platform-aws';
