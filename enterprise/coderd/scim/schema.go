// Package scim implements a SCIM 2.0 server for Coder users.
//
// It wraps github.com/elimity-com/scim with a Coder-specific
// ResourceHandler that creates, reads, and updates users via the
// Coder database, plus middleware for header-based authentication
// and audit logging.
//
// See RFC 7643 (Core Schema) and RFC 7644 (Protocol).
package scim

import (
	"github.com/elimity-com/scim"
	"github.com/elimity-com/scim/optional"
	"github.com/elimity-com/scim/schema"
)

// userResourceType returns the SCIM ResourceType for /Users backed by the
// given handler.
//
// We use the prebuilt core User schema. Fields Coder does not yet honor
// (e.g. phoneNumbers, addresses) are accepted by the validator but
// silently ignored by the handler. That matches the spec's tolerance
// rules and is friendlier to IdPs than rejecting unknown attributes.
func userResourceType(handler scim.ResourceHandler) scim.ResourceType {
	return scim.ResourceType{
		ID:          optional.NewString("User"),
		Name:        "User",
		Endpoint:    "/Users",
		Description: optional.NewString("User Account"),
		Schema:      schema.CoreUserSchema(),
		Handler:     handler,
	}
}

// serviceProviderConfig describes the capabilities of Coder's SCIM
// implementation. The values mirror what the legacy
// /ServiceProviderConfig endpoint advertised so existing IdP
// integrations see no functional change in capabilities.
func serviceProviderConfig() scim.ServiceProviderConfig {
	return scim.ServiceProviderConfig{
		DocumentationURI: optional.NewString("https://coder.com/docs/admin/users/oidc-auth#scim"),
		SupportPatch:     true,
		AuthenticationSchemes: []scim.AuthenticationScheme{
			{
				Type:             scim.AuthenticationTypeOauthBearerToken,
				Name:             "HTTP Header Authentication",
				Description:      "Authentication scheme using the Authorization header with the shared token",
				DocumentationURI: optional.NewString("https://coder.com/docs/admin/users/oidc-auth#scim"),
			},
		},
	}
}
