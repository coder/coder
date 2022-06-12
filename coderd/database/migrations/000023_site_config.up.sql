CREATE TABLE IF NOT EXISTS site_config (
    key varchar(256) NOT NULL UNIQUE,
    value varchar(8192) NOT NULL
);
