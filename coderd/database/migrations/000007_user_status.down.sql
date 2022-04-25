DROP TYPE user_status_type;

ALTER TABLE ONLY users
    DROP COLUMN IF EXISTS status;
