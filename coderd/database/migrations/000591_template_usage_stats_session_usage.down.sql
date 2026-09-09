-- The fixed columns are NOT NULL with no default, so add them with a default
-- to fill existing rows, then drop the default to restore the original schema.
ALTER TABLE template_usage_stats
	ADD COLUMN ssh_mins smallint DEFAULT 0 NOT NULL,
	ADD COLUMN sftp_mins smallint DEFAULT 0 NOT NULL,
	ADD COLUMN reconnecting_pty_mins smallint DEFAULT 0 NOT NULL,
	ADD COLUMN vscode_mins smallint DEFAULT 0 NOT NULL,
	ADD COLUMN jetbrains_mins smallint DEFAULT 0 NOT NULL;

ALTER TABLE template_usage_stats
	ALTER COLUMN ssh_mins DROP DEFAULT,
	ALTER COLUMN sftp_mins DROP DEFAULT,
	ALTER COLUMN reconnecting_pty_mins DROP DEFAULT,
	ALTER COLUMN vscode_mins DROP DEFAULT,
	ALTER COLUMN jetbrains_mins DROP DEFAULT;

COMMENT ON COLUMN template_usage_stats.ssh_mins IS 'Total minutes the user has been using SSH.';

COMMENT ON COLUMN template_usage_stats.sftp_mins IS 'Total minutes the user has been using SFTP.';

COMMENT ON COLUMN template_usage_stats.reconnecting_pty_mins IS 'Total minutes the user has been using the reconnecting PTY.';

COMMENT ON COLUMN template_usage_stats.vscode_mins IS 'Total minutes the user has been using VSCode.';

COMMENT ON COLUMN template_usage_stats.jetbrains_mins IS 'Total minutes the user has been using JetBrains.';

-- Restore the five families the fixed columns have room for. Usage attributed
-- to any other family is discarded, and so is all per-app session usage: the
-- fixed columns have nowhere to put either.
UPDATE template_usage_stats AS tus
SET
	ssh_mins = families.ssh_mins,
	sftp_mins = families.sftp_mins,
	reconnecting_pty_mins = families.reconnecting_pty_mins,
	vscode_mins = families.vscode_mins,
	jetbrains_mins = families.jetbrains_mins
FROM (
	SELECT
		start_time,
		template_id,
		user_id,
		COALESCE(MAX(usage_mins) FILTER (WHERE family = 'ssh'), 0)::smallint AS ssh_mins,
		COALESCE(MAX(usage_mins) FILTER (WHERE family = 'sftp'), 0)::smallint AS sftp_mins,
		COALESCE(MAX(usage_mins) FILTER (WHERE family = 'reconnecting_pty'), 0)::smallint AS reconnecting_pty_mins,
		COALESCE(MAX(usage_mins) FILTER (WHERE family = 'vscode'), 0)::smallint AS vscode_mins,
		COALESCE(MAX(usage_mins) FILTER (WHERE family = 'jetbrains'), 0)::smallint AS jetbrains_mins
	FROM
		template_usage_stats_session_families
	WHERE
		family IN ('ssh', 'sftp', 'reconnecting_pty', 'vscode', 'jetbrains')
	GROUP BY
		start_time, template_id, user_id
) AS families
WHERE
	tus.start_time = families.start_time
	AND tus.template_id = families.template_id
	AND tus.user_id = families.user_id;

DROP TABLE template_usage_stats_session_apps;

DROP TABLE template_usage_stats_session_families;
