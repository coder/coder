ALTER TYPE audit_action ADD VALUE IF NOT EXISTS 'start';
ALTER TYPE audit_action ADD VALUE IF NOT EXISTS 'stop';

ALTER TYPE resource_type ADD VALUE IF NOT EXISTS 'workspace_build';
