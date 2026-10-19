CREATE FUNCTION delete_chat_project_user_acl_on_org_member_delete() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    -- Drop the departing member's direct grants so adding them back to the
    -- organization does not silently restore access.
    UPDATE chat_projects
    SET
        user_acl = user_acl - OLD.user_id::text,
        updated_at = now()
    WHERE organization_id = OLD.organization_id AND user_acl ? OLD.user_id::text;
    RETURN OLD;
END;
$$;

CREATE TRIGGER trigger_delete_chat_project_user_acl_on_org_member_delete
    BEFORE DELETE ON organization_members
    FOR EACH ROW
    EXECUTE FUNCTION delete_chat_project_user_acl_on_org_member_delete();

-- Drop grants for users who already left the project's organization.
UPDATE chat_projects
SET user_acl = (
    SELECT COALESCE(jsonb_object_agg(entry.key, entry.value), '{}'::jsonb)
    FROM jsonb_each(chat_projects.user_acl) AS entry
    WHERE EXISTS (
        SELECT 1
        FROM organization_members
        WHERE organization_members.organization_id = chat_projects.organization_id
            AND organization_members.user_id::text = entry.key
    )
)
WHERE user_acl <> '{}'::jsonb;
