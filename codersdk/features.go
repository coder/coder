package codersdk

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
