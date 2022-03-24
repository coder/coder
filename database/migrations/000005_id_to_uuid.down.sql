BEGIN;

LOCK TABLE users;
LOCK TABLE organizations;

UPDATE
    metric_events
SET
    user_id = users._id_old,
    organization_id = organizations._id_old
FROM
    users
LEFT JOIN
    organizations
ON
    true
WHERE
    metric_events.user_id = users.id::text AND
    metric_events.organization_id = organizations.id::text;

--
-- organizations
--

LOCK TABLE organization_members;
LOCK TABLE org_templates;
LOCK TABLE provider_org_whitelist;
LOCK TABLE services;

ALTER TABLE organization_members
    DROP CONSTRAINT organization_members_organization_id_fkey;
ALTER TABLE org_templates
    DROP CONSTRAINT org_templates_organization_id_fkey;
ALTER TABLE provider_org_whitelist
    DROP CONSTRAINT whitelisted_namespaces_org_id_fkey;
ALTER TABLE services
    DROP CONSTRAINT services_organization_id_fkey;


ALTER TABLE organizations ALTER COLUMN id TYPE text USING id::text;

ALTER TABLE organization_members ALTER COLUMN organization_id TYPE text USING organization_id::text;
ALTER TABLE org_templates ALTER COLUMN organization_id TYPE text USING organization_id::text;
ALTER TABLE provider_org_whitelist ALTER COLUMN org_id TYPE text USING org_id::text;
ALTER TABLE services ALTER COLUMN organization_id TYPE text USING organization_id::text;

UPDATE
    organization_members
SET
    organization_id = orgs._id_old
FROM
    organizations orgs
WHERE
    organization_members.organization_id = orgs.id;

UPDATE
    org_templates
SET
    organization_id = orgs._id_old
FROM
    organizations orgs
WHERE
    org_templates.organization_id = orgs.id;

UPDATE
    provider_org_whitelist
SET
    org_id = orgs._id_old
FROM
    organizations orgs
WHERE
    provider_org_whitelist.org_id = orgs.id;

UPDATE
    services
SET
    organization_id = orgs._id_old
FROM
    organizations orgs
WHERE
    services.organization_id = orgs.id;

UPDATE
    registries
SET
    organization_id = orgs._id_old
FROM
    organizations orgs
WHERE
    registries.organization_id = orgs.id;

UPDATE
    environments
SET
    organization_id = orgs._id_old
FROM
    organizations orgs
WHERE
    environments.organization_id = orgs.id;

UPDATE
    images
SET
    organization_id = orgs._id_old
FROM
    organizations orgs
WHERE
    images.organization_id = orgs.id;

UPDATE organizations SET id = _id_old;
ALTER TABLE organizations DROP COLUMN _id_old;

ALTER TABLE ONLY organization_members
    ADD CONSTRAINT organization_members_organization_id_fkey FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE;
ALTER TABLE ONLY org_templates
    ADD CONSTRAINT org_templates_organization_id_fkey FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE;
ALTER TABLE ONLY provider_org_whitelist
    ADD CONSTRAINT whitelisted_namespaces_org_id_fkey FOREIGN KEY (org_id) REFERENCES organizations(id) ON DELETE CASCADE;
ALTER TABLE ONLY services
    ADD CONSTRAINT services_organization_id_fkey FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE;

--
-- users
--

LOCK TABLE autostart_records;
LOCK TABLE organization_members;

ALTER TABLE autostart_records
    DROP CONSTRAINT autostart_records_user_id_fkey;

ALTER TABLE organization_members
    DROP CONSTRAINT organization_members_user_id_fkey;

ALTER TABLE users ALTER COLUMN id TYPE text USING id::text;
ALTER TABLE autostart_records ALTER COLUMN user_id TYPE text USING user_id::text;
ALTER TABLE organization_members ALTER COLUMN user_id TYPE text USING user_id::text;

UPDATE
    organization_members
SET
    user_id = users._id_old
FROM
    users
WHERE
    organization_members.user_id = users.id;

UPDATE
    autostart_records
SET
    user_id = users._id_old
FROM
    users
WHERE
    autostart_records.user_id = users.id;

UPDATE
    api_keys
SET
    user_id = users._id_old
FROM
    users
WHERE
    api_keys.user_id = users.id;

UPDATE
    audit_logs
SET
    user_id = users._id_old
FROM
    users
WHERE
    audit_logs.user_id = users.id;

UPDATE
    dev_urls
SET
    user_id = users._id_old
FROM
    users
WHERE
    dev_urls.user_id = users.id;

UPDATE
    environments
SET
    user_id = users._id_old
FROM
    users
WHERE
    environments.user_id = users.id;

UPDATE
    oauth_links
SET
    user_id = users._id_old
FROM
    users
WHERE
    oauth_links.user_id = users.id;

UPDATE
    oauth_states
SET
    user_id = users._id_old
FROM
    users
WHERE
    oauth_states.user_id = users.id;

UPDATE
    user_flags
SET
    user_id = users._id_old
FROM
    users
WHERE
    user_flags.user_id = users.id;

UPDATE users SET id = _id_old;
ALTER TABLE users DROP COLUMN _id_old;

ALTER TABLE ONLY autostart_records
    ADD CONSTRAINT autostart_records_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY organization_members
    ADD CONSTRAINT organization_members_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

COMMIT;
