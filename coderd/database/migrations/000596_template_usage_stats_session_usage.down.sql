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

-- Fold app names into the five families the fixed columns have room for,
-- capped at the half hour a bucket covers. SQL cannot call into Go, so this
-- copies codersdk.appNameFamilies. An app registered after this migration has
-- no entry here, so its minutes are dropped, as are the per-app detail and any
-- family the fixed columns cannot hold.
UPDATE template_usage_stats AS tus
SET
	ssh_mins = families.ssh_mins,
	sftp_mins = families.sftp_mins,
	reconnecting_pty_mins = families.reconnecting_pty_mins,
	vscode_mins = families.vscode_mins,
	jetbrains_mins = families.jetbrains_mins
FROM (
	SELECT
		apps.start_time,
		apps.template_id,
		apps.user_id,
		LEAST(COALESCE(SUM(apps.usage_mins) FILTER (WHERE registry.family = 'ssh'), 0), 30)::smallint AS ssh_mins,
		LEAST(COALESCE(SUM(apps.usage_mins) FILTER (WHERE registry.family = 'sftp'), 0), 30)::smallint AS sftp_mins,
		LEAST(COALESCE(SUM(apps.usage_mins) FILTER (WHERE registry.family = 'reconnecting_pty'), 0), 30)::smallint AS reconnecting_pty_mins,
		LEAST(COALESCE(SUM(apps.usage_mins) FILTER (WHERE registry.family = 'vscode'), 0), 30)::smallint AS vscode_mins,
		LEAST(COALESCE(SUM(apps.usage_mins) FILTER (WHERE registry.family = 'jetbrains'), 0), 30)::smallint AS jetbrains_mins
	FROM
		template_usage_stats_session_apps AS apps
	JOIN (VALUES
		('ssh', 'ssh'),
		('zed', 'ssh'),
		('sftp', 'sftp'),
		('reconnecting_pty', 'reconnecting_pty'),
		('jetbrains', 'jetbrains'),
		('vscode', 'vscode'),
		('vscode_insiders', 'vscode'),
		('vscode_web', 'vscode'),
		('code_server', 'vscode'),
		('cursor', 'vscode'),
		('windsurf', 'vscode'),
		('positron', 'vscode'),
		('vscodium', 'vscode'),
		('codium', 'vscode'),
		('antigravity', 'vscode'),
		('trae', 'vscode'),
		('kiro', 'vscode'),
		('devin', 'vscode')
	) AS registry(app, family)
	ON
		registry.app = apps.app_name
	GROUP BY
		apps.start_time, apps.template_id, apps.user_id
) AS families
WHERE
	tus.start_time = families.start_time
	AND tus.template_id = families.template_id
	AND tus.user_id = families.user_id;

DROP TABLE template_usage_stats_session_apps;

ALTER TABLE template_usage_stats
	DROP COLUMN session_usage_digest;
