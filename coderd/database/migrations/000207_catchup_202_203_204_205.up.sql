--------------------------------------------------------------------------------
-- 000202_remove_max_ttl.up.sql
--------------------------------------------------------------------------------
-- Update the template_with_users view by recreating it.
DROP VIEW template_with_users;

ALTER TABLE templates DROP COLUMN IF EXISTS "max_ttl";
ALTER TABLE templates DROP COLUMN IF EXISTS "use_max_ttl";

CREATE VIEW
        template_with_users
AS
SELECT
        templates.*,
        coalesce(visible_users.avatar_url, '') AS created_by_avatar_url,
        coalesce(visible_users.username, '') AS created_by_username
FROM
        templates
                LEFT JOIN
        visible_users
        ON
                templates.created_by = visible_users.id;
COMMENT ON VIEW template_with_users IS 'Joins in the username + avatar url of the created by user.';

--------------------------------------------------------------------------------
-- 000203_template_usage_stats.up.sql
--------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS template_usage_stats (
  start_time timestamptz NOT NULL,
  end_time timestamptz NOT NULL,
  template_id uuid NOT NULL,
  user_id uuid NOT NULL,
  median_latency_ms real NULL,
  usage_mins smallint NOT NULL,
  ssh_mins smallint NOT NULL,
  sftp_mins smallint NOT NULL,
  reconnecting_pty_mins smallint NOT NULL,
  vscode_mins smallint NOT NULL,
  jetbrains_mins smallint NOT NULL,
  app_usage_mins jsonb NULL,

  PRIMARY KEY (start_time, template_id, user_id)
);

COMMENT ON TABLE template_usage_stats IS 'Records aggregated usage statistics for templates/users. All usage is rounded up to the nearest minute.';
COMMENT ON COLUMN template_usage_stats.start_time IS 'Start time of the usage period.';
COMMENT ON COLUMN template_usage_stats.end_time IS 'End time of the usage period.';
COMMENT ON COLUMN template_usage_stats.template_id IS 'ID of the template being used.';
COMMENT ON COLUMN template_usage_stats.user_id IS 'ID of the user using the template.';
COMMENT ON COLUMN template_usage_stats.median_latency_ms IS 'Median latency the user is experiencing, in milliseconds. Null means no value was recorded.';
COMMENT ON COLUMN template_usage_stats.usage_mins IS 'Total minutes the user has been using the template.';
COMMENT ON COLUMN template_usage_stats.ssh_mins IS 'Total minutes the user has been using SSH.';
COMMENT ON COLUMN template_usage_stats.sftp_mins IS 'Total minutes the user has been using SFTP.';
COMMENT ON COLUMN template_usage_stats.reconnecting_pty_mins IS 'Total minutes the user has been using the reconnecting PTY.';
COMMENT ON COLUMN template_usage_stats.vscode_mins IS 'Total minutes the user has been using VSCode.';
COMMENT ON COLUMN template_usage_stats.jetbrains_mins IS 'Total minutes the user has been using JetBrains.';
COMMENT ON COLUMN template_usage_stats.app_usage_mins IS 'Object with app names as keys and total minutes used as values. Null means no app usage was recorded.';

CREATE UNIQUE INDEX IF NOT EXISTS template_usage_stats_start_time_template_id_user_id_idx ON template_usage_stats (start_time, template_id, user_id);
CREATE INDEX IF NOT EXISTS template_usage_stats_start_time_idx ON template_usage_stats  (start_time DESC);

COMMENT ON INDEX template_usage_stats_start_time_template_id_user_id_idx IS 'Index for primary key.';
COMMENT ON INDEX template_usage_stats_start_time_idx IS 'Index for querying MAX(start_time).';

--------------------------------------------------------------------------------
-- 000204_add_workspace_agent_scripts_fk_index.up.sql
--------------------------------------------------------------------------------
CREATE INDEX IF NOT EXISTS workspace_agent_scripts_workspace_agent_id_idx ON workspace_agent_scripts (workspace_agent_id);

COMMENT ON INDEX workspace_agent_scripts_workspace_agent_id_idx IS 'Foreign key support index for faster lookups';

--------------------------------------------------------------------------------
-- 000205_unique_linked_id.up.sql
--------------------------------------------------------------------------------
-- Remove the linked_id if two user_links share the same value.
-- This will affect the user if they attempt to change their settings on
-- the oauth/oidc provider. However, if two users exist with the same
-- linked_value, there is no way to determine correctly which user should
-- be updated. Since the linked_id is empty, this value will be linked
-- by email.
UPDATE ONLY user_links AS out
SET
        linked_id =
                CASE WHEN (
                          -- When the count of linked_id is greater than 1, set the linked_id to empty
                          SELECT
                              COUNT(*)
                          FROM
                              user_links inn
                          WHERE
                              out.linked_id = inn.linked_id AND out.login_type = inn.login_type
                  ) > 1 THEN '' ELSE out.linked_id END;

-- Enforce unique linked_id constraint on non-empty linked_id
CREATE UNIQUE INDEX IF NOT EXISTS user_links_linked_id_login_type_idx ON user_links USING btree (linked_id, login_type) WHERE (linked_id != '');
