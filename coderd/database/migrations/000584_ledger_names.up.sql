-- A ledger's name carries no model, because it serves every model of its
-- subject. See "A journal is named for its model, a ledger only for its
-- subject" in poc_audit/implementation_patterns.md.
--
-- The credential is what forced this: its use is about to be recorded into the
-- same row its lifecycle posts to, and a name claiming the lifecycle would then
-- be wrong. The other two are renamed for consistency rather than necessity,
-- since a ledger naming one model is wrong the moment a second appears and
-- there is no reason to wait for that.

ALTER TABLE credential_lifecycle_ledger RENAME TO credential_ledger;
ALTER TABLE ai_agent_lifecycle_ledger RENAME TO ai_agent_ledger;
ALTER TABLE authorization_lifecycle_ledger RENAME TO authorization_ledger;

-- Renaming a table leaves its constraints and indexes under the old name.
ALTER TABLE credential_ledger
    RENAME CONSTRAINT credential_lifecycle_ledger_state TO credential_ledger_state;
ALTER TABLE credential_ledger
    RENAME CONSTRAINT credential_lifecycle_ledger_pkey TO credential_ledger_pkey;
ALTER INDEX credential_lifecycle_ledger_holder_idx RENAME TO credential_ledger_holder_idx;

ALTER TABLE ai_agent_ledger
    RENAME CONSTRAINT ai_agent_lifecycle_ledger_state TO ai_agent_ledger_state;
ALTER TABLE ai_agent_ledger
    RENAME CONSTRAINT ai_agent_lifecycle_ledger_pkey TO ai_agent_ledger_pkey;
ALTER INDEX ai_agent_lifecycle_ledger_owner_idx RENAME TO ai_agent_ledger_owner_idx;

ALTER TABLE authorization_ledger
    RENAME CONSTRAINT authorization_lifecycle_ledger_scope_reserved TO authorization_ledger_scope_reserved;
ALTER TABLE authorization_ledger
    RENAME CONSTRAINT authorization_lifecycle_ledger_state TO authorization_ledger_state;
ALTER TABLE authorization_ledger
    RENAME CONSTRAINT authorization_lifecycle_ledger_pkey TO authorization_ledger_pkey;
ALTER INDEX authorization_lifecycle_ledger_agent_idx RENAME TO authorization_ledger_agent_idx;
ALTER INDEX authorization_lifecycle_ledger_principal_idx RENAME TO authorization_ledger_principal_idx;
