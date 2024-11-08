package enidpsync

import (
	"context"
	"net/http"

	"github.com/golang-jwt/jwt/v4"

	"github.com/coder/coder/v2/coderd/idpsync"
	"github.com/coder/coder/v2/codersdk"
)

func (e EnterpriseIDPSync) GroupSyncEntitled() bool {
	return e.entitlements.Enabled(codersdk.FeatureTemplateRBAC)
}

// ParseGroupClaims parses the user claims and handles deployment wide group behavior.
// Almost all behavior is deferred since each organization configures it's own
// group sync settings.
// GroupAllowList is implemented here to prevent login by unauthorized users.
// TODO: GroupAllowList overlaps with the default organization group sync settings.
func (e EnterpriseIDPSync) ParseGroupClaims(ctx context.Context, mergedClaims jwt.MapClaims) (idpsync.GroupParams, *idpsync.HTTPError) {
	if !e.GroupSyncEntitled() {
		return e.AGPLIDPSync.ParseGroupClaims(ctx, mergedClaims)
	}

	if e.GroupField != "" && len(e.GroupAllowList) > 0 {
		groupsRaw, ok := mergedClaims[e.GroupField]
		if !ok {
			return idpsync.GroupParams{}, &idpsync.HTTPError{
				Code:             http.StatusForbidden,
				Msg:              "Not a member of an allowed group",
				Detail:           "You have no groups in your claims!",
				RenderStaticPage: true,
			}
		}
		parsedGroups, err := idpsync.ParseStringSliceClaim(groupsRaw)
		if err != nil {
			return idpsync.GroupParams{}, &idpsync.HTTPError{
				Code:             http.StatusBadRequest,
				Msg:              "Failed read groups from claims for allow list check. Ask an administrator for help.",
				Detail:           err.Error(),
				RenderStaticPage: true,
			}
		}

		inAllowList := false
	AllowListCheckLoop:
		for _, group := range parsedGroups {
			if _, ok := e.GroupAllowList[group]; ok {
				inAllowList = true
				break AllowListCheckLoop
			}
		}

		if !inAllowList {
			return idpsync.GroupParams{}, &idpsync.HTTPError{
				Code:             http.StatusForbidden,
				Msg:              "Not a member of an allowed group",
				Detail:           "Ask an administrator to add one of your groups to the allow list.",
				RenderStaticPage: true,
			}
		}
	}

	return idpsync.GroupParams{
		SyncEntitled: true,
		MergedClaims: mergedClaims,
	}, nil
}
