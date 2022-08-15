package codersdk

import (
	"context"
	"encoding/json"
	"net/http"
)

type Entitlement string

const (
	EntitlementEntitled    Entitlement = "entitled"
	EntitlementGracePeriod Entitlement = "grace_period"
	EntitlementNotEntitled Entitlement = "not_entitled"
)

const (
	FeatureUserLimit = "user_limit"
	FeatureAuditLog  = "audit_log"
)

var FeatureNames = []string{FeatureUserLimit, FeatureAuditLog}

type Feature struct {
	Entitlement Entitlement `json:"entitlement"`
	Enabled     bool        `json:"enabled"`
	Limit       *int64      `json:"limit"`
	Actual      *int64      `json:"actual"`
}

type Entitlements struct {
	Features   map[string]Feature `json:"features"`
	Warnings   []string           `json:"warnings"`
	HasLicense bool               `json:"has_license"`
}

type EntitlementsRequest struct {
	// placeholder so that we can add request parameters in future
	// without breaking changes to the go API
}

func (c *Client) Entitlements(ctx context.Context, _ EntitlementsRequest) (Entitlements, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/entitlements", nil)
	if err != nil {
		return Entitlements{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return Entitlements{}, readBodyAsError(res)
	}
	var ent Entitlements
	return ent, json.NewDecoder(res.Body).Decode(&ent)
}
