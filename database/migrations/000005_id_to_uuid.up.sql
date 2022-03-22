--
-- users
--

ALTER TABLE users ADD COLUMN id_uuid uuid NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';

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

CREATE UNIQUE INDEX idx_users_id_uuid ON users USING btree (id_uuid);

--
-- organizations
--

ALTER TABLE organizations ADD COLUMN id_uuid uuid NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';

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
    organizations.id = raw.id;

CREATE UNIQUE INDEX idx_organizations_id_uuid ON organizations USING btree (id_uuid);

--
-- organization members
--

ALTER TABLE organization_members ADD COLUMN user_id_uuid uuid NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';
ALTER TABLE organization_members ADD COLUMN organization_id_uuid uuid NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';

UPDATE 
    organization_members
SET
    user_id_uuid = users.id_uuid
FROM
    users
WHERE
    organization_members.user_id = users.id;

UPDATE 
    organization_members
SET
    organization_id_uuid = orgs.id_uuid
FROM
    organizations orgs
WHERE
    organization_members.user_id = orgs.id;

CREATE UNIQUE INDEX idx_organization_members_id_uuid ON organization_members USING btree (user_id_uuid, organization_id_uuid);
