package coderd

import (
	"net/http"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
)

// listScopeCatalog returns the curated catalog of API key scopes that users
// can request. The catalog groups scopes into legacy “specials”, low-level
// resource:action atoms, and composite coder:* scopes.
//
// @Summary List API key scopes
// @ID list-api-key-scopes
// @Tags Authorization
// @Produce json
// @Success 200 {object} codersdk.ScopeCatalog
// @Router /auth/scopes [get]
func (*API) listScopeCatalog(rw http.ResponseWriter, r *http.Request) {
	lowMeta := rbac.ExternalLowLevelCatalog()
	low := make([]codersdk.ScopeCatalogLowLevel, 0, len(lowMeta))
	for _, meta := range lowMeta {
		low = append(low, codersdk.ScopeCatalogLowLevel{
			Name:     codersdk.APIKeyScope(meta.Name),
			Resource: codersdk.RBACResource(meta.Resource),
			Action:   string(meta.Action),
		})
	}

	compMeta := rbac.ExternalCompositeCatalog()
	composites := make([]codersdk.ScopeCatalogComposite, 0, len(compMeta))
	for _, meta := range compMeta {
		expands := make([]codersdk.APIKeyScope, 0, len(meta.ExpandsTo))
		for _, name := range meta.ExpandsTo {
			expands = append(expands, codersdk.APIKeyScope(name))
		}
		composites = append(composites, codersdk.ScopeCatalogComposite{
			Name:      codersdk.APIKeyScope(meta.Name),
			ExpandsTo: expands,
		})
	}

	specials := rbac.ExternalSpecialScopes()
	specialScopes := make([]codersdk.APIKeyScope, 0, len(specials))
	for _, name := range specials {
		specialScopes = append(specialScopes, codersdk.APIKeyScope(name))
	}

	httpapi.Write(r.Context(), rw, http.StatusOK, codersdk.ScopeCatalog{
		Specials:   specialScopes,
		LowLevel:   low,
		Composites: composites,
	})
}
