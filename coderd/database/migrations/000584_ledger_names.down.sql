ALTER INDEX authorization_ledger_principal_idx RENAME TO authorization_lifecycle_ledger_principal_idx;
ALTER INDEX authorization_ledger_agent_idx RENAME TO authorization_lifecycle_ledger_agent_idx;
ALTER TABLE authorization_ledger
    RENAME CONSTRAINT authorization_ledger_pkey TO authorization_lifecycle_ledger_pkey;
ALTER TABLE authorization_ledger
    RENAME CONSTRAINT authorization_ledger_state TO authorization_lifecycle_ledger_state;
ALTER TABLE authorization_ledger
    RENAME CONSTRAINT authorization_ledger_scope_reserved TO authorization_lifecycle_ledger_scope_reserved;

ALTER INDEX ai_agent_ledger_owner_idx RENAME TO ai_agent_lifecycle_ledger_owner_idx;
ALTER TABLE ai_agent_ledger
    RENAME CONSTRAINT ai_agent_ledger_pkey TO ai_agent_lifecycle_ledger_pkey;
ALTER TABLE ai_agent_ledger
    RENAME CONSTRAINT ai_agent_ledger_state TO ai_agent_lifecycle_ledger_state;

ALTER INDEX credential_ledger_holder_idx RENAME TO credential_lifecycle_ledger_holder_idx;
ALTER TABLE credential_ledger
    RENAME CONSTRAINT credential_ledger_pkey TO credential_lifecycle_ledger_pkey;
ALTER TABLE credential_ledger
    RENAME CONSTRAINT credential_ledger_state TO credential_lifecycle_ledger_state;

ALTER TABLE authorization_ledger RENAME TO authorization_lifecycle_ledger;
ALTER TABLE ai_agent_ledger RENAME TO ai_agent_lifecycle_ledger;
ALTER TABLE credential_ledger RENAME TO credential_lifecycle_ledger;
