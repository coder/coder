-- API key scopes for the delegated_identity RBAC resource.
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'delegated_identity:*';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'delegated_identity:create';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'delegated_identity:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'delegated_identity:update';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'delegated_identity:delete';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'delegated_identity:use';

ALTER TYPE resource_type ADD VALUE IF NOT EXISTS 'delegated_identity';

CREATE TABLE delegated_identities (
    id uuid PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    organization_id uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    parent_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    root_user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_by uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name text NOT NULL,
    scopes text[] NOT NULL DEFAULT '{}',
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT delegated_identities_not_own_parent CHECK (parent_id <> id)
);

COMMENT ON TABLE delegated_identities IS 'A delegated identity is the identity of something that acts under a user, such as a coding agent. It is an account of its own: it owns what it creates and is named in audit logs. It can never do more than its parent.';
COMMENT ON COLUMN delegated_identities.id IS 'The account that is this identity. The row in users is a service account with no way to sign in.';
COMMENT ON COLUMN delegated_identities.parent_id IS 'The user or delegated identity this identity was created under. It never changes.';
COMMENT ON COLUMN delegated_identities.root_user_id IS 'The user at the top of the chain of parents. The identity is judged with this user''s roles and groups, and its spend counts against this user.';
COMMENT ON COLUMN delegated_identities.created_by IS 'Who created the identity. It differs from the parent when a trusted service creates an identity under a member.';
COMMENT ON COLUMN delegated_identities.name IS 'The username the account was created with.';
COMMENT ON COLUMN delegated_identities.scopes IS 'API key scope names. An empty list grants nothing. What the identity may do is these scopes, less whatever a parent identity lacks.';

CREATE INDEX delegated_identities_parent_id_idx ON delegated_identities (parent_id);
CREATE INDEX delegated_identities_root_user_id_idx ON delegated_identities (root_user_id);
