ALTER TABLE organizations
    ADD COLUMN members_can_disable_skills boolean NOT NULL DEFAULT false;

COMMENT ON COLUMN organizations.members_can_disable_skills IS 'Whether organization members can turn off organization skills for their own chats. While false, member choices are kept but ignored.';

-- One row per organization member who has saved an organization skill
-- choice. Removing the membership deletes the row.
CREATE TABLE organization_member_skill_settings (
    organization_id     uuid    NOT NULL,
    user_id             uuid    NOT NULL,
    all_skills_disabled boolean NOT NULL DEFAULT false,
    PRIMARY KEY (organization_id, user_id),
    FOREIGN KEY (organization_id, user_id)
        REFERENCES organization_members (organization_id, user_id) ON DELETE CASCADE
);

COMMENT ON TABLE organization_member_skill_settings IS 'Per-member organization skill settings. Applied only while organizations.members_can_disable_skills is true.';

COMMENT ON COLUMN organization_member_skill_settings.all_skills_disabled IS 'Whether the member turned off every organization skill, including skills added later. Per-skill choices in organization_member_disabled_skills are kept and apply again when this is false.';

-- Organization skills a member turned off for their own chats.
CREATE TABLE organization_member_disabled_skills (
    organization_id uuid NOT NULL,
    user_id         uuid NOT NULL,
    skill_id        uuid NOT NULL REFERENCES skills (id) ON DELETE CASCADE,
    PRIMARY KEY (organization_id, user_id, skill_id),
    FOREIGN KEY (organization_id, user_id)
        REFERENCES organization_members (organization_id, user_id) ON DELETE CASCADE
);

COMMENT ON TABLE organization_member_disabled_skills IS 'Organization skills that a member turned off for their own chats. Applied only while organizations.members_can_disable_skills is true.';

-- Supports the cascade from skills, which the primary key cannot serve.
CREATE INDEX idx_organization_member_disabled_skills_skill_id
    ON organization_member_disabled_skills (skill_id);
