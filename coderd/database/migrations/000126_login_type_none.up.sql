ALTER TYPE login_type ADD VALUE IF NOT EXISTS 'none';

COMMENT ON TYPE login_type IS 'Specifies the method of authentication. "none" is a special case in which no authentication method is allowed.';
