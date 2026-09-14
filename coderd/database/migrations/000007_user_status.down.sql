ALTER TABLE ONLY users
    DROP COLUMN IF EXISTS status;

DROP TYPE user_status;
