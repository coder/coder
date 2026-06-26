-- Add 'copilot' to ai_provider_type. The aibridge runtime already supports
-- Copilot via aibridge.NewCopilotProvider; the enum just needs the
-- discriminator so DB-driven providers can carry it. Mirrors the precedent
-- in 000499_ai_provider_type_chatd_values.up.sql.
ALTER TYPE ai_provider_type ADD VALUE IF NOT EXISTS 'copilot';
