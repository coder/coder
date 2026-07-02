-- Classifies the scan root that discovered a context resource. Mirrors
-- the agent's OriginKind proto enum. 'unspecified' is reserved for rows
-- written by pre-origin agents that did not report a kind.
CREATE TYPE workspace_agent_context_origin_kind AS ENUM (
    'unspecified',
    'working_dir',
    'builtin',
    'user_source'
);

-- workspace_agent_context_resources: rename source_path to origin_root
-- (now recorded for every scan root, not only user-declared ones) and
-- add origin_kind classifying that root.
ALTER TABLE workspace_agent_context_resources
    RENAME COLUMN source_path TO origin_root;

ALTER TABLE workspace_agent_context_resources
    ADD COLUMN origin_kind workspace_agent_context_origin_kind NOT NULL DEFAULT 'unspecified';

-- Backfill: before this migration a non-empty path was only ever a
-- user-declared source, so classify those rows accordingly. Built-in and
-- working-directory roots left an empty path and stay unspecified.
UPDATE workspace_agent_context_resources
    SET origin_kind = 'user_source'
    WHERE origin_root <> '';

COMMENT ON COLUMN workspace_agent_context_resources.origin_root IS 'Filesystem path of the scan root that discovered this resource: the working directory, a built-in root, or a user-declared source. Empty only for rows written by pre-origin agents.';
COMMENT ON COLUMN workspace_agent_context_resources.origin_kind IS 'Classifies origin_root as working_dir, builtin, or user_source. unspecified only for rows written by pre-origin agents that did not report a kind.';

-- chat_context_resources mirrors the agent table; apply the same rename,
-- column, backfill, and comments so pinned chat copies carry provenance.
ALTER TABLE chat_context_resources
    RENAME COLUMN source_path TO origin_root;

ALTER TABLE chat_context_resources
    ADD COLUMN origin_kind workspace_agent_context_origin_kind NOT NULL DEFAULT 'unspecified';

UPDATE chat_context_resources
    SET origin_kind = 'user_source'
    WHERE origin_root <> '';

COMMENT ON COLUMN chat_context_resources.origin_root IS 'Filesystem path of the scan root that discovered this resource: the working directory, a built-in root, or a user-declared source. Empty only for rows written by pre-origin agents.';
COMMENT ON COLUMN chat_context_resources.origin_kind IS 'Classifies origin_root as working_dir, builtin, or user_source. unspecified only for rows written by pre-origin agents that did not report a kind.';
