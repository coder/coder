package codersdk

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

// PremiumFunnelSource identifies the premium feature whose paywall the user
// interacted with. This is a fixed enum rather than a page URL because paywall
// routes embed organization and template names.
type PremiumFunnelSource string

const (
	PremiumFunnelSourceAIBridgeSessionThreads PremiumFunnelSource = "aibridge_session_threads"
	PremiumFunnelSourceAIBridgeSessions       PremiumFunnelSource = "aibridge_sessions"
	PremiumFunnelSourceAIGatewayKeys          PremiumFunnelSource = "ai_gateway_keys"
	PremiumFunnelSourceAIGovernance           PremiumFunnelSource = "ai_governance"
	PremiumFunnelSourceAppearance             PremiumFunnelSource = "appearance"
	PremiumFunnelSourceAuditLog               PremiumFunnelSource = "audit_log"
	PremiumFunnelSourceBrowserOnly            PremiumFunnelSource = "browser_only"
	PremiumFunnelSourceConnectionLog          PremiumFunnelSource = "connection_log"
	PremiumFunnelSourceCustomRoles            PremiumFunnelSource = "custom_roles"
	PremiumFunnelSourceGroups                 PremiumFunnelSource = "groups"
	PremiumFunnelSourceIdpOrgSync             PremiumFunnelSource = "idp_org_sync"
	PremiumFunnelSourceIdpSync                PremiumFunnelSource = "idp_sync"
	PremiumFunnelSourceMultipleOrganizations  PremiumFunnelSource = "multiple_organizations"
	PremiumFunnelSourceObservability          PremiumFunnelSource = "observability"
	PremiumFunnelSourceProvisionerKeys        PremiumFunnelSource = "provisioner_keys"
	PremiumFunnelSourceProvisioners           PremiumFunnelSource = "provisioners"
	PremiumFunnelSourceTemplatePermissions    PremiumFunnelSource = "template_permissions"
	PremiumFunnelSourceWorkspaceProxies       PremiumFunnelSource = "workspace_proxies"
	// PremiumFunnelSourceDirect is reported when a trial is requested without
	// passing through a paywall, such as from the sidebar or a bookmark. It
	// keeps "arrived without attribution" distinguishable from missing data.
	PremiumFunnelSourceDirect PremiumFunnelSource = "direct"
)

// PremiumFunnelSources returns every valid funnel source.
func PremiumFunnelSources() []PremiumFunnelSource {
	return []PremiumFunnelSource{
		PremiumFunnelSourceAIBridgeSessionThreads,
		PremiumFunnelSourceAIBridgeSessions,
		PremiumFunnelSourceAIGatewayKeys,
		PremiumFunnelSourceAIGovernance,
		PremiumFunnelSourceAppearance,
		PremiumFunnelSourceAuditLog,
		PremiumFunnelSourceBrowserOnly,
		PremiumFunnelSourceConnectionLog,
		PremiumFunnelSourceCustomRoles,
		PremiumFunnelSourceGroups,
		PremiumFunnelSourceIdpOrgSync,
		PremiumFunnelSourceIdpSync,
		PremiumFunnelSourceMultipleOrganizations,
		PremiumFunnelSourceObservability,
		PremiumFunnelSourceProvisionerKeys,
		PremiumFunnelSourceProvisioners,
		PremiumFunnelSourceTemplatePermissions,
		PremiumFunnelSourceWorkspaceProxies,
		PremiumFunnelSourceDirect,
	}
}

// Valid reports whether the source is a known funnel source.
func (s PremiumFunnelSource) Valid() bool {
	for _, source := range PremiumFunnelSources() {
		if s == source {
			return true
		}
	}
	return false
}

// PremiumFunnelVariant identifies which paywall presentation was rendered.
type PremiumFunnelVariant string

const (
	PremiumFunnelVariantPremium      PremiumFunnelVariant = "premium"
	PremiumFunnelVariantSmall        PremiumFunnelVariant = "small"
	PremiumFunnelVariantAIGovernance PremiumFunnelVariant = "ai_governance"
)

// Valid reports whether the variant is a known paywall presentation.
func (v PremiumFunnelVariant) Valid() bool {
	switch v {
	case PremiumFunnelVariantPremium, PremiumFunnelVariantSmall, PremiumFunnelVariantAIGovernance:
		return true
	default:
		return false
	}
}

// PremiumFunnelEventRequest is the request body for
// POST /api/v2/deployment/premium-funnel-events.
type PremiumFunnelEventRequest struct {
	// ID identifies this click, and doubles as the attribution token that a
	// later trial signup reports.
	ID      uuid.UUID            `json:"id" format:"uuid" validate:"required"`
	Source  PremiumFunnelSource  `json:"source" validate:"required"`
	Variant PremiumFunnelVariant `json:"variant" validate:"required"`
}

// ReportPremiumFunnelEvent reports a click on a premium paywall call to action
// for telemetry purposes. It is a no-op on deployments with telemetry
// disabled. Trial signups are reported by coderd when a license is actually
// issued, so a client cannot forge a conversion.
func (c *Client) ReportPremiumFunnelEvent(ctx context.Context, req PremiumFunnelEventRequest) error {
	res, err := c.Request(ctx, http.MethodPost, "/api/v2/deployment/premium-funnel-events", req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}
