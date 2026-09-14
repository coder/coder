-- Set a cap of 7 days on template max_ttl
--
-- NOTE: this migration was added in August 2022, but the cap has been changed
--       to 30 days in July 2023.
UPDATE templates SET max_ttl = 604800000000000 WHERE max_ttl > 604800000000000;
