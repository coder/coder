package coderd

import (
	"net/http"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/enterprise/coderd/scim"
)

// newSCIMHandler constructs the SCIM 2.0 HTTP handler for /scim/v2.
//
// The handler implements the user resource per RFC 7644 and uses the
// elimity-com/scim framework for request routing, schema validation,
// PATCH op parsing, and discovery endpoints (/Schemas,
// /ResourceTypes, /ServiceProviderConfig).
//
// Callers should mount the returned handler under /scim/v2 and gate it
// with the SCIM feature middleware.
func (api *API) newSCIMHandler() (http.Handler, error) {
	h, err := scim.New(scim.Options{
		Database:   api.Database,
		Logger:     api.Logger.Named("scim"),
		Auditor:    &api.AGPL.Auditor,
		IDPSync:    api.IDPSync,
		APIKey:     api.SCIMAPIKey,
		CreateUser: api.AGPL.CreateUser,
	})
	if err != nil {
		return nil, xerrors.Errorf("build scim handler: %w", err)
	}
	return h, nil
}
