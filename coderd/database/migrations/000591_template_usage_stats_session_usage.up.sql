-- The primary keys put user_id before template_id: the insights read caps a
-- user's minutes per half hour across templates, and looks up one user's rows
-- in a half hour through this prefix. The upsert's conflict target names the
-- same columns in the parent's order, which the unique index satisfies.
CREATE TABLE template_usage_stats_session_families (
	start_time timestamptz NOT NULL,
	template_id uuid NOT NULL,
	user_id uuid NOT NULL,
	family text NOT NULL,
	usage_mins smallint NOT NULL,

	PRIMARY KEY (start_time, user_id, template_id, family),
	FOREIGN KEY (start_time, template_id, user_id)
		REFERENCES template_usage_stats (start_time, template_id, user_id)
		ON DELETE CASCADE
);

COMMENT ON TABLE template_usage_stats_session_families IS 'Session usage of each template_usage_stats bucket, split by app family. A bucket with no row here recorded no session usage.';

COMMENT ON COLUMN template_usage_stats_session_families.family IS 'Family name the registry attributed the session to when the bucket was last rolled up, including ''unknown'' for an app name the registry did not know. Buckets the rollup no longer revisits keep their recorded attribution.';

COMMENT ON COLUMN template_usage_stats_session_families.usage_mins IS 'Total minutes the user has been using the family. Minutes shared by two apps of the family count once.';

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

COMMENT ON TABLE template_usage_stats_session_apps IS 'Session usage of each template_usage_stats bucket, split by app name. A bucket with family rows but no rows here predates per-app recording, so its per-app usage is unknown rather than zero.';

COMMENT ON COLUMN template_usage_stats_session_apps.app_name IS 'App name as the agent reported it, so it is a source label rather than a curated identity. An agent that reports only the fixed session counts reports family names here, as does history converted by migration 000590.';

COMMENT ON COLUMN template_usage_stats_session_apps.usage_mins IS 'Total minutes the user has been using the app.';

-- Carry every family the fixed columns recorded, sftp included: the rollup has
-- never written it, but a row that has a value must not lose it. Zero minutes
-- are skipped so a bucket has rows only for the families it saw, which is what
-- the rollup writes from now on. No app rows are written: the fixed columns
-- only ever recorded the family, so per-app usage stays unknown for these
-- buckets rather than being invented from family totals.
INSERT INTO template_usage_stats_session_families (start_time, template_id, user_id, family, usage_mins)
SELECT
	tus.start_time,
	tus.template_id,
	tus.user_id,
	families.family,
	families.usage_mins
FROM
	template_usage_stats AS tus,
	LATERAL (VALUES
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
