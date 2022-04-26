DROP TYPE user_status;

ALTER TABLE ONLY users
    DROP COLUMN IF EXISTS status;
