CREATE TYPE user_status_type AS ENUM ('active', 'suspended');

ALTER TABLE ONLY users
    ADD COLUMN IF NOT EXISTS status user_status_type NOT NULL DEFAULT 'active';
