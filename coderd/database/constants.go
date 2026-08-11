package database

import (
	"github.com/google/uuid"

	"github.com/coder/coder/v2/codersdk"
)

// PrebuildsSystemUserID mirrors codersdk.PrebuildsSystemUserID, parsed
// for use as a uuid.UUID. Both must agree; tests pin the value to the
// codersdk constant so the two cannot drift.
var PrebuildsSystemUserID = uuid.MustParse(codersdk.PrebuildsSystemUserID)

// OAuth2ScopeUnrestricted is the oauth2_provider_app_codes.scope and
// oauth2_provider_app_tokens.scope value recording a grant that carries no
// restriction. Both columns hold space-separated values from the
// api_key_scope vocabulary, so an unrestricted grant is spelled the same way
// api_keys.scopes spells it. The columns are NOT NULL: writing this constant
// is how a caller states "unrestricted" on purpose, which is what
// distinguishes a deliberate grant from a scope that was never threaded
// through.
const OAuth2ScopeUnrestricted = string(ApiKeyScopeCoderAll)
