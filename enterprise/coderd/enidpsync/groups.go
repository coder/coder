package enidpsync

import (
	"context"

	"github.com/golang-jwt/jwt/v4"

	"github.com/coder/coder/v2/coderd/idpsync"
	"github.com/coder/coder/v2/codersdk"
)

func (e EnterpriseIDPSync) GroupSyncEnabled() bool {
	return e.entitlements.Enabled(codersdk.FeatureTemplateRBAC)

}

// ParseGroupClaims parses the user claims and handles deployment wide group behavior.
// Almost all behavior is deferred since each organization configures it's own
// group sync settings.
// TODO: Implement group allow_list behavior here since that is deployment wide.
func (e EnterpriseIDPSync) ParseGroupClaims(ctx context.Context, mergedClaims jwt.MapClaims) (idpsync.GroupParams, *idpsync.HTTPError) {
	if !e.GroupSyncEnabled() {
		return e.AGPLIDPSync.ParseGroupClaims(ctx, mergedClaims)
	}

	return idpsync.GroupParams{
		SyncEnabled:  e.OrganizationSyncEnabled(),
		MergedClaims: mergedClaims,
	}, nil
}
