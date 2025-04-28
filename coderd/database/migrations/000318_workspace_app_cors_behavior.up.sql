CREATE TYPE app_cors_behavior AS ENUM (
    'simple',
    'passthru'
);

-- https://www.postgresql.org/docs/16/sql-altertable.html
-- When a column is added with ADD COLUMN and a non-volatile DEFAULT is specified, the default is evaluated at the time
-- of the statement and the result stored in the table's metadata. That value will be used for the column for all existing rows.
ALTER TABLE workspace_apps
    ADD COLUMN cors_behavior app_cors_behavior NOT NULL DEFAULT 'simple'::app_cors_behavior;