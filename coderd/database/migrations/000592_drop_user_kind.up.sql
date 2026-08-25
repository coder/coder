-- An AI agent stops having a users row, and users stops having a kind.
--
-- The row was the last of the mirror. It survived the `ai_agents` table because
-- code still routed on `users.kind = 'ai_agent'` and because a name was read
-- from it; neither is true now. Nothing structural held it: `ai_agent_ledger`
-- has no foreign key to `users`, the ledger having minted the identifier.
--
-- See "A system actor is stored as a user because there was nowhere else to put
-- it" in poc_audit/entity_model.md, which is the same shape and is not fixed
-- here.

-- **An interception's initiator is a party, and not every party is a user.**
-- For an AI agent's key the initiator is the agent, so this reference cannot
-- point into `users` once an agent has no row there. Dropping it is the change
-- `api_keys.holder_id` already took and for the same reason: the column names
-- whoever acted.
--
-- What is given up is the database's guarantee that an initiator exists.
-- Reconciling interceptions against the two ledgers is what replaces it, and
-- nothing does that yet.
ALTER TABLE aibridge_interceptions
    DROP CONSTRAINT aibridge_interceptions_initiator_id_fkey;

-- `user_status_change_trigger` writes a row for every user at insert, and that
-- reference restricts rather than cascading, so these go before the rows they
-- name. An AI agent's status never meant anything: it was written `active` at
-- creation and never moved.
DELETE FROM user_status_changes
WHERE user_id IN (SELECT id FROM users WHERE kind = 'ai_agent'::user_kind);

DELETE FROM user_deleted
WHERE user_id IN (SELECT id FROM users WHERE kind = 'ai_agent'::user_kind);

DELETE FROM users WHERE kind = 'ai_agent'::user_kind;

-- **The rows go before the filters do, and the order is load bearing.**
-- Seventeen queries filter `kind = 'human'` and one excludes `'ai_agent'`.
-- Removing a filter while agent rows still existed would hand those agents
-- group membership, organization membership, roles, insights and AI seats.
-- Deleting the rows first makes every filter vacuous, so removing them is a
-- no-op rather than a change of behaviour.
--
-- `GetAuthorizationUserRoles` is the one worth naming: its filter is what
-- enforced "an AI agent has no roles of its own" in SQL. With no users row the
-- query returns no row anyway, for a better reason.
ALTER TABLE users DROP COLUMN kind;

DROP TYPE user_kind;

-- **Dropping the column took two checks with it that are not about AI agents.**
-- Postgres drops any constraint depending on a dropped column, and both of
-- these named `kind` only to exempt an agent from a rule meant for people. They
-- are restored without that exemption, which makes them stricter than they
-- were: there is no longer a kind of user they do not apply to.
--
-- Their old forms were:
--   users_email_not_empty
--     CHECK ((email = '') = (is_service_account = true OR kind = 'ai_agent'))
--   users_service_account_login_type
--     CHECK ((is_service_account = false AND kind <> 'ai_agent') OR login_type = 'none')
ALTER TABLE users
    ADD CONSTRAINT users_email_not_empty
        CHECK ((email = '') = (is_service_account = true)),
    ADD CONSTRAINT users_service_account_login_type
        CHECK ((is_service_account = false) OR (login_type = 'none'::login_type));
