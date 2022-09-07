ALTER TABLE users
    ADD COLUMN deleted boolean DEFAULT false NOT NULL;
