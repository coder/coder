CREATE TYPE workspace_build_deadline_reason AS ENUM (
	'unknown',
	'build',
	'activity',
	'user_changed',
	'autostop_disabled',
	'template_schedule'
);

COMMENT ON TYPE workspace_build_deadline_reason IS 'Why a build deadline last changed. Only the column default and the invalidation trigger write unknown.';

-- Writers cast their reason through this type, so they cannot record unknown.
CREATE TYPE workspace_build_deadline_writer_reason AS ENUM (
	'build',
	'activity',
	'user_changed',
	'autostop_disabled',
	'template_schedule'
);

COMMENT ON TYPE workspace_build_deadline_writer_reason IS 'The deadline reasons that queries can write. Keep equal to workspace_build_deadline_reason without unknown.';

ALTER TABLE workspace_builds
	ADD COLUMN deadline_reason workspace_build_deadline_reason NOT NULL DEFAULT 'unknown',
	ADD COLUMN deadline_changed_at timestamp with time zone,
	ADD COLUMN deadline_changed_by uuid REFERENCES users(id) ON DELETE SET NULL,
	ADD COLUMN deadline_sources jsonb NOT NULL DEFAULT '[]'::jsonb,
	ADD CONSTRAINT workspace_builds_deadline_change_check CHECK (
		(deadline_changed_at IS NOT NULL OR deadline_reason = 'unknown')
		AND (deadline_reason != 'unknown' OR (deadline_changed_by IS NULL AND deadline_sources = '[]'::jsonb))
		AND (deadline_reason = 'activity' OR deadline_sources = '[]'::jsonb)
	),
	ADD CONSTRAINT workspace_builds_deadline_sources_check CHECK (jsonb_typeof(deadline_sources) = 'array');

COMMENT ON COLUMN workspace_builds.deadline_reason IS 'Why deadline last changed. unknown means Coder has no record of the cause.';
COMMENT ON COLUMN workspace_builds.deadline_changed_at IS 'When deadline last changed. NULL only for unknown changes without a known time.';
COMMENT ON COLUMN workspace_builds.deadline_changed_by IS 'User who caused the last deadline change, when known.';
COMMENT ON COLUMN workspace_builds.deadline_sources IS 'Typed activity sources of the last deadline change. Empty unless deadline_reason is activity.';

-- The actor of a build deadline is the user who started the build. Automatic
-- builds, prebuilds, and missing users have no actor.
CREATE FUNCTION workspace_build_deadline_build_actor(initiator_id uuid, build_reason build_reason) RETURNS uuid AS $$
	SELECT users.id
	FROM users
	WHERE users.id = initiator_id
		AND initiator_id != 'c42fdf75-3097-471c-8c33-fb52454d81c0'::uuid
		AND build_reason IN ('initiator', 'dashboard', 'cli', 'ssh_connection', 'vscode_connection', 'jetbrains_connection');
$$ LANGUAGE sql STABLE;

-- A writer that changes deadline without writing a new snapshot is a previous
-- server version. Record that the cause is unknown, and when it happened.
CREATE FUNCTION invalidate_workspace_build_deadline_change() RETURNS trigger AS $$
BEGIN
	IF NEW.deadline IS DISTINCT FROM OLD.deadline
		AND NEW.deadline_reason IS NOT DISTINCT FROM OLD.deadline_reason
		AND NEW.deadline_changed_at IS NOT DISTINCT FROM OLD.deadline_changed_at
		AND NEW.deadline_changed_by IS NOT DISTINCT FROM OLD.deadline_changed_by
		AND NEW.deadline_sources IS NOT DISTINCT FROM OLD.deadline_sources THEN
		NEW.deadline_reason := 'unknown';
		NEW.deadline_changed_at := NOW();
		NEW.deadline_changed_by := NULL;
		NEW.deadline_sources := '[]'::jsonb;
	END IF;
	RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER invalidate_workspace_build_deadline_change
	BEFORE UPDATE OF deadline ON workspace_builds
	FOR EACH ROW EXECUTE FUNCTION invalidate_workspace_build_deadline_change();

DROP VIEW workspace_build_with_user;

CREATE VIEW workspace_build_with_user AS
 SELECT workspace_builds.id,
    workspace_builds.created_at,
    workspace_builds.updated_at,
    workspace_builds.workspace_id,
    workspace_builds.template_version_id,
    workspace_builds.build_number,
    workspace_builds.transition,
    workspace_builds.initiator_id,
    workspace_builds.job_id,
    workspace_builds.deadline,
    workspace_builds.reason,
    workspace_builds.daily_cost,
    workspace_builds.max_deadline,
    workspace_builds.template_version_preset_id,
    workspace_builds.has_external_agent,
    workspace_builds.notified_autostop_deadline,
    workspace_builds.deadline_reason,
    workspace_builds.deadline_changed_at,
    workspace_builds.deadline_changed_by,
    workspace_builds.deadline_sources,
    COALESCE(visible_users.avatar_url, ''::text) AS initiator_by_avatar_url,
    COALESCE(visible_users.username, ''::text) AS initiator_by_username,
    COALESCE(visible_users.name, ''::text) AS initiator_by_name,
    COALESCE(deadline_actor.avatar_url, ''::text) AS deadline_changed_by_avatar_url,
    COALESCE(deadline_actor.username, ''::text) AS deadline_changed_by_username,
    COALESCE(deadline_actor.name, ''::text) AS deadline_changed_by_name
   FROM workspace_builds
     LEFT JOIN visible_users ON workspace_builds.initiator_id = visible_users.id
     LEFT JOIN visible_users AS deadline_actor ON workspace_builds.deadline_changed_by = deadline_actor.id;

COMMENT ON VIEW workspace_build_with_user IS 'Joins build initiator and deadline actor presentation.';
