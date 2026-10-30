package coderd

import (
	"net/http"

	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/telemetry"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary Report a premium paywall click
// @Description Records through telemetry that a user selected the call to action on a Premium paywall.
// @Description The dashboard calls this when a user selects a paywall button, so you don't need to call it.
// @Description Requires permission to read licenses, and returns `404` without it.
// @ID report-a-premium-paywall-click
// @Security CoderSessionToken
// @Accept json
// @Tags Enterprise
// @Param request body codersdk.PremiumFunnelEventRequest true "Paywall event to record. `id` identifies the event, and a later trial request can pass it as `attribution_id`. `source` is the feature that the paywall is for, and `variant` is the paywall presentation, which is `premium`, `small`, or `ai_governance`."
// @Success 204
// @Router /api/v2/deployment/premium-funnel-events [post]
func (api *API) postPremiumFunnelEvent(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)

	// Reports are authorized rather than trusted: a few paywalls show their
	// call to action to everyone, so a user who cannot read licenses can click
	// one, and their click must not enter the funnel.
	if !api.Authorize(r, policy.ActionRead, rbac.ResourceLicense) {
		httpapi.ResourceNotFound(rw)
		return
	}

	var req codersdk.PremiumFunnelEventRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	var validations []codersdk.ValidationError
	if !req.Source.Valid() {
		validations = append(validations, codersdk.ValidationError{
			Field:  "source",
			Detail: "Unknown premium funnel source.",
		})
	}
	if !req.Variant.Valid() {
		validations = append(validations, codersdk.ValidationError{
			Field:  "variant",
			Detail: "Unknown premium paywall variant.",
		})
	}
	if len(validations) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid premium funnel event.",
			Validations: validations,
		})
		return
	}

	api.Telemetry.Report(&telemetry.Snapshot{
		PremiumFunnelEvents: []telemetry.PremiumFunnelEvent{
			{
				ID:        req.ID,
				EventType: telemetry.PremiumFunnelEventCTAClick,
				Source:    string(req.Source),
				Variant:   string(req.Variant),
				UserID:    apiKey.UserID,
				CreatedAt: dbtime.Now(),
			},
		},
	})

	rw.WriteHeader(http.StatusNoContent)
}
