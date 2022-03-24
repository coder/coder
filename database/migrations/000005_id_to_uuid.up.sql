--
-- users
--

BEGIN;

ALTER TABLE users ADD COLUMN id_uuid uuid NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';
ALTER TABLE users ADD COLUMN _id_old text NOT NULL DEFAULT '';
UPDATE users SET _id_old = id;

LOCK TABLE users;
LOCK TABLE autostart_records;
LOCK TABLE organization_members;

ALTER TABLE autostart_records
    DROP CONSTRAINT autostart_records_user_id_fkey;

ALTER TABLE organization_members
    DROP CONSTRAINT organization_members_user_id_fkey;

UPDATE USERS SET id_uuid = 'e4398d91-8797-4009-aabf-bd116a373088'::uuid WHERE id = 'admin';

UPDATE
    users
SET
    id_uuid =   (encode(substring(raw.id_raw, 1, 4), 'hex') || '-' ||
                encode(substring(raw.id_raw, 5, 2), 'hex') || '-' ||
                to_hex(((right(substring(raw.id_raw, 7, 1)::text, -1)::varbit & 'x0f') | 'x40')::int) ||
                encode(substring(raw.id_raw, 8, 1), 'hex') || '-' ||
                to_hex(((right(substring(raw.id_raw, 9, 1)::text, -1)::varbit & 'x3f') | 'x80')::int) ||
                encode(substring(raw.id_raw, 10, 1), 'hex') || '-' ||
                encode(substring(raw.id_raw, 11), 'hex'))::uuid
FROM (
    SELECT
        id,
        decode(replace(id, '-', ''), 'hex') AS id_raw
    FROM
        users
) AS raw
WHERE
    users.id = raw.id AND
    users.id != 'admin';

UPDATE
    organization_members
SET
    user_id = users.id_uuid
FROM
    users
WHERE
    organization_members.user_id = users.id;

UPDATE
    autostart_records
SET
    user_id = users.id_uuid
FROM
    users
WHERE
    autostart_records.user_id = users.id;

UPDATE
    api_keys
SET
    user_id = users.id_uuid
FROM
    users
WHERE
    api_keys.user_id = users.id;

UPDATE
    audit_logs
SET
    user_id = users.id_uuid
FROM
    users
WHERE
    audit_logs.user_id = users.id;

UPDATE
    dev_urls
SET
    user_id = users.id_uuid
FROM
    users
WHERE
    dev_urls.user_id = users.id;

UPDATE
    environments
SET
    user_id = users.id_uuid
FROM
    users
WHERE
    environments.user_id = users.id;

UPDATE
    oauth_links
SET
    user_id = users.id_uuid
FROM
    users
WHERE
    oauth_links.user_id = users.id;

UPDATE
    oauth_states
SET
    user_id = users.id_uuid
FROM
    users
WHERE
    oauth_states.user_id = users.id;

UPDATE
    user_flags
SET
    user_id = users.id_uuid
FROM
    users
WHERE
    user_flags.user_id = users.id;

UPDATE USERS SET id = id_uuid;

ALTER TABLE users ALTER COLUMN id TYPE uuid USING id::uuid;
ALTER TABLE autostart_records ALTER COLUMN user_id TYPE uuid USING user_id::uuid;
ALTER TABLE organization_members ALTER COLUMN user_id TYPE uuid USING user_id::uuid;

ALTER TABLE ONLY autostart_records
    ADD CONSTRAINT autostart_records_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY organization_members
    ADD CONSTRAINT organization_members_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;


--
-- organizations
--

ALTER TABLE organizations ADD COLUMN id_uuid uuid NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';
ALTER TABLE organizations ADD COLUMN _id_old text NOT NULL DEFAULT '';
UPDATE organizations SET _id_old = id;

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

UPDATE organizations SET id_uuid = 'e4398d91-8797-4009-aabf-bd116a373088'::uuid WHERE id = 'default';

UPDATE
    organizations
SET
    id_uuid =   (encode(substring(raw.id_raw, 1, 4), 'hex') || '-' ||
                encode(substring(raw.id_raw, 5, 2), 'hex') || '-' ||
                to_hex(((right(substring(raw.id_raw, 7, 1)::text, -1)::varbit & 'x0f') | 'x40')::int) ||
                encode(substring(raw.id_raw, 8, 1), 'hex') || '-' ||
                to_hex(((right(substring(raw.id_raw, 9, 1)::text, -1)::varbit & 'x3f') | 'x80')::int) ||
                encode(substring(raw.id_raw, 10, 1), 'hex') || '-' ||
                encode(substring(raw.id_raw, 11), 'hex'))::uuid
FROM (
    SELECT
        id,
        decode(replace(id, '-', ''), 'hex') AS id_raw
    FROM
        organizations
) AS raw
WHERE
    organizations.id = raw.id AND
    organizations.id != 'default';

UPDATE
    organization_members
SET
    organization_id = orgs.id_uuid
FROM
    organizations orgs
WHERE
    organization_members.organization_id = orgs.id;

UPDATE
    org_templates
SET
    organization_id = orgs.id_uuid
FROM
    organizations orgs
WHERE
    org_templates.organization_id = orgs.id;

UPDATE
    provider_org_whitelist
SET
    org_id = orgs.id_uuid
FROM
    organizations orgs
WHERE
    provider_org_whitelist.org_id = orgs.id;

UPDATE
    services
SET
    organization_id = orgs.id_uuid
FROM
    organizations orgs
WHERE
    services.organization_id = orgs.id;

UPDATE
    registries
SET
    organization_id = orgs.id_uuid
FROM
    organizations orgs
WHERE
    registries.organization_id = orgs.id;

UPDATE
    environments
SET
    organization_id = orgs.id_uuid
FROM
    organizations orgs
WHERE
    environments.organization_id = orgs.id;

UPDATE
    images
SET
    organization_id = orgs.id_uuid
FROM
    organizations orgs
WHERE
    images.organization_id = orgs.id;

UPDATE organizations SET id = id_uuid;

ALTER TABLE organizations ALTER COLUMN id TYPE uuid USING id::uuid;

ALTER TABLE organization_members ALTER COLUMN organization_id TYPE uuid USING organization_id::uuid;
ALTER TABLE org_templates ALTER COLUMN organization_id TYPE uuid USING organization_id::uuid;
ALTER TABLE provider_org_whitelist ALTER COLUMN org_id TYPE uuid USING org_id::uuid;
ALTER TABLE services ALTER COLUMN organization_id TYPE uuid USING organization_id::uuid;

ALTER TABLE ONLY organization_members
    ADD CONSTRAINT organization_members_organization_id_fkey FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE;
ALTER TABLE ONLY org_templates
    ADD CONSTRAINT org_templates_organization_id_fkey FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE;
ALTER TABLE ONLY provider_org_whitelist
    ADD CONSTRAINT whitelisted_namespaces_org_id_fkey FOREIGN KEY (org_id) REFERENCES organizations(id) ON DELETE CASCADE;
ALTER TABLE ONLY services
    ADD CONSTRAINT services_organization_id_fkey FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE;

--
-- we do metric events at the end because both the user_id and org_id was
-- updated
--

UPDATE
    metric_events
SET
    user_id = users.id_uuid,
    organization_id = organizations.id_uuid
FROM
    users
LEFT JOIN
    organizations
ON
    true
WHERE
    metric_events.user_id = users._id_old AND
    metric_events.organization_id = organizations._id_old;

ALTER TABLE users DROP COLUMN id_uuid;
ALTER TABLE organizations DROP COLUMN id_uuid;

COMMIT;
