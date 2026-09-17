-- user_id precedes template_id so the insights read, which caps a user's
-- minutes per half hour across templates, can scan one user's rows by prefix.
CREATE TABLE template_usage_stats_session_apps (
	start_time timestamptz NOT NULL,
	template_id uuid NOT NULL,
	user_id uuid NOT NULL,
	app_name text NOT NULL,
	usage_mins smallint NOT NULL,

	PRIMARY KEY (start_time, user_id, template_id, app_name),
	FOREIGN KEY (start_time, template_id, user_id)
		REFERENCES template_usage_stats (start_time, template_id, user_id)
		ON DELETE CASCADE
);

COMMENT ON TABLE template_usage_stats_session_apps IS 'Session usage of each template_usage_stats bucket, split by app name. No row means the bucket recorded no session usage. Reads group app names into families through the codersdk registry.';

COMMENT ON COLUMN template_usage_stats_session_apps.app_name IS 'App name as the agent reported it, so a source label rather than a curated identity. Rows converted from the fixed session columns carry a family name here instead.';

COMMENT ON COLUMN template_usage_stats_session_apps.usage_mins IS 'Total minutes the user has been using the app. A minute counts once however many sessions were open.';

-- The fixed columns recorded families, so these rows carry a family name,
-- which the registry maps back to itself. sftp is included: the rollup never
-- writes it, but recorded minutes must not be lost.
INSERT INTO template_usage_stats_session_apps (start_time, template_id, user_id, app_name, usage_mins)
SELECT
	tus.start_time,
	tus.template_id,
	tus.user_id,
	families.family,
	families.usage_mins
FROM
	template_usage_stats AS tus
CROSS JOIN LATERAL (VALUES
	('ssh', tus.ssh_mins),
	('sftp', tus.sftp_mins),
	('reconnecting_pty', tus.reconnecting_pty_mins),
	('vscode', tus.vscode_mins),
	('jetbrains', tus.jetbrains_mins)
) AS families(family, usage_mins)
WHERE
	families.usage_mins > 0;

ALTER TABLE template_usage_stats
	DROP COLUMN ssh_mins,
	DROP COLUMN sftp_mins,
	DROP COLUMN reconnecting_pty_mins,
	DROP COLUMN vscode_mins,
	DROP COLUMN jetbrains_mins;
