-- This has to be outside a transaction
ALTER TYPE resource_type ADD VALUE IF NOT EXISTS 'health_settings';
