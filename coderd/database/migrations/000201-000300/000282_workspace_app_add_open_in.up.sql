CREATE TYPE workspace_app_open_in AS ENUM ('tab', 'window', 'slim-window');

ALTER TABLE workspace_apps ADD COLUMN open_in workspace_app_open_in NOT NULL DEFAULT 'slim-window'::workspace_app_open_in;
