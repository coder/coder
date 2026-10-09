CREATE TABLE agent_hours_organization_allotments (
    organization_id UUID PRIMARY KEY REFERENCES organizations(id) ON DELETE CASCADE,
    -- Share in basis points (10000 = 100%).
    allotment_bps   INTEGER     NOT NULL CHECK (allotment_bps > 0 AND allotment_bps <= 10000),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE agent_hours_organization_allotments IS 'Share of the deployment''s licensed Agent Hours allotted to an organization, in basis points. Configuration only; not enforced.';

CREATE TABLE agent_hours_group_allotments (
    group_id      UUID PRIMARY KEY REFERENCES groups(id) ON DELETE CASCADE,
    -- Share in basis points (10000 = 100%).
    allotment_bps INTEGER     NOT NULL CHECK (allotment_bps > 0 AND allotment_bps <= 10000),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE agent_hours_group_allotments IS 'Share of the group''s organization''s Agent Hours allotted to a group, in basis points. Configuration only; not enforced.';

ALTER TYPE resource_type ADD VALUE IF NOT EXISTS 'agent_hours_organization_allotment';
ALTER TYPE resource_type ADD VALUE IF NOT EXISTS 'agent_hours_group_allotment';
