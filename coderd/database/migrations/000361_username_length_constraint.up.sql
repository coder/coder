ALTER TABLE users
ADD CONSTRAINT users_username_min_length
CHECK (length(username) >= 1);
