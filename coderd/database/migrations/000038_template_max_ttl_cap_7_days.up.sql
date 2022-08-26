-- Set a cap of 7 days on template max_ttl
UPDATE templates SET max_ttl = 604800000000000 WHERE max_ttl > 604800000000000;
