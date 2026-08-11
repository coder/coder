package database

import (
	"github.com/google/uuid"

	"github.com/coder/coder/v2/codersdk"
)

// PrebuildsSystemUserID mirrors codersdk.PrebuildsSystemUserID, parsed
// for use as a uuid.UUID. Both must agree; tests pin the value to the
// codersdk constant so the two cannot drift.
var PrebuildsSystemUserID = uuid.MustParse(codersdk.PrebuildsSystemUserID)

// Values stored in oauth2_provider_apps.client_type, as plain strings for
// comparison against the nullable column.
//
// Converted from the codersdk constants rather than redeclared, so the value
// registration writes and the value OAuth2ProviderApp.IsPublic reads back
// cannot disagree. That divergence would fail closed anyway (the app would read
// as confidential and demand a secret it was never issued), but it would fail
// visibly to a client rather than here.
//
// What this does not protect against is the two constants colliding on the same
// value, which would make IsPublic true for confidential apps. Nothing in the
// type system can catch that; the tests that pin these spellings to the wire
// values do, so do not delete them as redundant:
// TestOAuth2ClientRegistrationRequest_DetermineClientType and
// TestCreateDynamicClientRegistration_ClientType.
const (
	OAuth2ProviderAppClientTypeConfidential = string(codersdk.OAuth2ClientTypeConfidential)
	OAuth2ProviderAppClientTypePublic       = string(codersdk.OAuth2ClientTypePublic)
)
