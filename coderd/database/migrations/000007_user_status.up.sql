CREATE TYPE user_status AS ENUM ('active', 'suspended');

ALTER TABLE ONLY users
    ADD COLUMN IF NOT EXISTS status user_status NOT NULL DEFAULT 'active';
