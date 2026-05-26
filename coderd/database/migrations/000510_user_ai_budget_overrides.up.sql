CREATE TABLE user_ai_budget_overrides (
    user_id            UUID        PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    group_id           UUID        NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    -- Spend limit applied to the user, in micro-units (1 unit = 1,000,000).
    spend_limit_micros BIGINT      NOT NULL CHECK (spend_limit_micros >= 0),
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
    -- The membership invariant (user must be a member of the attributed
    -- group, including when that group is "Everyone") would naturally be
    -- a composite FK to group_members_expanded, but PostgreSQL does not
    -- allow FKs to views. It's enforced instead by a conditional INSERT
    -- in UpsertUserAIBudgetOverride and triggers on the underlying
    -- membership tables.
);

COMMENT ON TABLE user_ai_budget_overrides IS 'Per-user AI spend override that supersedes group budget resolution.';

-- When a user is removed from a regular group, delete any override
-- attributed to that group.
CREATE FUNCTION delete_user_ai_budget_overrides_on_group_member_delete() RETURNS TRIGGER
	LANGUAGE plpgsql
AS $$
BEGIN
	DELETE FROM user_ai_budget_overrides
	WHERE user_id = OLD.user_id AND group_id = OLD.group_id;
	RETURN OLD;
END;
$$;

CREATE TRIGGER trigger_delete_user_ai_budget_overrides_on_group_member_delete
	BEFORE DELETE ON group_members
	FOR EACH ROW
EXECUTE PROCEDURE delete_user_ai_budget_overrides_on_group_member_delete();

-- When a user is removed from an organization, delete any override
-- attributed to that organization's "Everyone" group (which has
-- id == organization_id).
CREATE FUNCTION delete_user_ai_budget_overrides_on_org_member_delete() RETURNS TRIGGER
	LANGUAGE plpgsql
AS $$
BEGIN
	DELETE FROM user_ai_budget_overrides
	WHERE user_id = OLD.user_id AND group_id = OLD.organization_id;
	RETURN OLD;
END;
$$;

CREATE TRIGGER trigger_delete_user_ai_budget_overrides_on_org_member_delete
	BEFORE DELETE ON organization_members
	FOR EACH ROW
EXECUTE PROCEDURE delete_user_ai_budget_overrides_on_org_member_delete();
