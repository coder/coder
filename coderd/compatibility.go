package coderd

import (
	"net/http"

	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/codersdk"
)

type Compatibility struct {
	Entitlements   codersdk.Entitlements
	WorkspaceQuota codersdk.WorkspaceQuota
}

func NewCompatibility(options *Options) *Compatibility {
	entitlements := codersdk.Entitlements{
		Features:     make(map[string]codersdk.Feature, len(codersdk.FeatureNames)),
		Warnings:     []string{},
		Errors:       []string{},
		HasLicense:   false,
		Experimental: options.DeploymentConfig.Experimental.Value,
		Trial:        false,
	}
	for _, v := range codersdk.FeatureNames {
		entitlements.Features[v] = codersdk.Feature{
			Entitlement: codersdk.EntitlementNotEntitled,
			Enabled:     false,
		}
	}
	return &Compatibility{
		Entitlements: entitlements,
		WorkspaceQuota: codersdk.WorkspaceQuota{
			CreditsConsumed: 0,
			Budget:          -1, // no license
		},
	}
}

// serveEntitlements return empty entitlements.
func (api *API) serveEntitlementsWithEmpty(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	httpapi.Write(ctx, rw, http.StatusOK, api.compatibility.Entitlements)
}

// workspaceQuota return empty quotas.
func (api *API) workspaceQuotaWithEmpty(rw http.ResponseWriter, r *http.Request) {
	if !api.Authorize(r, rbac.ActionRead, rbac.ResourceUser) {
		httpapi.ResourceNotFound(rw)
		return
	}
	httpapi.Write(r.Context(), rw, http.StatusOK, api.compatibility.WorkspaceQuota)
}
