ALTER TYPE audit_action
  ADD VALUE IF NOT EXISTS 'login';

ALTER TYPE audit_action
  ADD VALUE IF NOT EXISTS 'logout';

ALTER TYPE resource_type
  ADD VALUE IF NOT EXISTS 'api_key';

