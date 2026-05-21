CREATE TABLE template_usage_stats (
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

CREATE UNIQUE INDEX ON template_usage_stats (start_time, template_id, user_id);
CREATE INDEX ON template_usage_stats (start_time DESC);

COMMENT ON INDEX template_usage_stats_start_time_template_id_user_id_idx IS 'Index for primary key.';
COMMENT ON INDEX template_usage_stats_start_time_idx IS 'Index for querying MAX(start_time).';
