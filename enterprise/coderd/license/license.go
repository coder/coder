package license

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/codersdk"
)

// Entitlements processes licenses to return whether features are enabled or not.
func Entitlements(ctx context.Context, db database.Store, logger slog.Logger, keys map[string]ed25519.PublicKey, enablements map[string]bool) (codersdk.Entitlements, error) {
	now := time.Now()
	// Default all entitlements to be disabled.
	entitlements := codersdk.Entitlements{
		Features: map[string]codersdk.Feature{},
		Warnings: []string{},
	}
	for _, featureName := range codersdk.FeatureNames {
		entitlements.Features[featureName] = codersdk.Feature{
			Entitlement: codersdk.EntitlementNotEntitled,
			Enabled:     enablements[featureName],
		}
	}

	licenses, err := db.GetUnexpiredLicenses(ctx)
	if err != nil {
		return entitlements, err
	}

	activeUserCount, err := db.GetActiveUserCount(ctx)
	if err != nil {
		return entitlements, xerrors.Errorf("query active user count: %w", err)
	}

	allFeatures := false

	// Here we loop through licenses to detect enabled features.
	for _, l := range licenses {
		claims, err := validateDBLicense(l, keys)
		if err != nil {
			logger.Debug(ctx, "skipping invalid license",
				slog.F("id", l.ID), slog.Error(err))
			continue
		}
		entitlements.HasLicense = true
		entitlement := codersdk.EntitlementEntitled
		entitlements.Trial = claims.Trial
		if now.After(claims.LicenseExpires.Time) {
			// if the grace period were over, the validation fails, so if we are after
			// LicenseExpires we must be in grace period.
			entitlement = codersdk.EntitlementGracePeriod
		}
		if claims.Features.UserLimit > 0 {
			entitlements.Features[codersdk.FeatureUserLimit] = codersdk.Feature{
				Enabled:     true,
				Entitlement: entitlement,
				Limit:       &claims.Features.UserLimit,
				Actual:      &activeUserCount,
			}
			if activeUserCount > claims.Features.UserLimit {
				entitlements.Warnings = append(entitlements.Warnings, fmt.Sprintf(
					"Your deployment has %d active users but is only licensed for %d.",
					activeUserCount, claims.Features.UserLimit))
			}
		}
		if claims.Features.AuditLog > 0 {
			entitlements.Features[codersdk.FeatureAuditLog] = codersdk.Feature{
				Entitlement: entitlement,
				Enabled:     enablements[codersdk.FeatureAuditLog],
			}
		}
		if claims.Features.BrowserOnly > 0 {
			entitlements.Features[codersdk.FeatureBrowserOnly] = codersdk.Feature{
				Entitlement: entitlement,
				Enabled:     enablements[codersdk.FeatureBrowserOnly],
			}
		}
		if claims.Features.SCIM > 0 {
			entitlements.Features[codersdk.FeatureSCIM] = codersdk.Feature{
				Entitlement: entitlement,
				Enabled:     enablements[codersdk.FeatureSCIM],
			}
		}
		if claims.Features.WorkspaceQuota > 0 {
			entitlements.Features[codersdk.FeatureWorkspaceQuota] = codersdk.Feature{
				Entitlement: entitlement,
				Enabled:     enablements[codersdk.FeatureWorkspaceQuota],
			}
		}
		if claims.AllFeatures {
			allFeatures = true
		}
	}

	if allFeatures {
		for _, featureName := range codersdk.FeatureNames {
			// No user limit!
			if featureName == codersdk.FeatureUserLimit {
				continue
			}
			feature := entitlements.Features[featureName]
			feature.Entitlement = codersdk.EntitlementEntitled
			entitlements.Features[featureName] = feature
		}
	}

	if entitlements.HasLicense {
		for _, featureName := range codersdk.FeatureNames {
			// The user limit has it's own warnings!
			if featureName == codersdk.FeatureUserLimit {
				continue
			}
			feature := entitlements.Features[featureName]
			if !feature.Enabled {
				continue
			}
			niceName := strings.Title(strings.ReplaceAll(featureName, "_", " "))
			switch feature.Entitlement {
			case codersdk.EntitlementNotEntitled:
				entitlements.Warnings = append(entitlements.Warnings,
					fmt.Sprintf("%s is enabled but your license is not entitled to this feature.", niceName))
				// Disable the feature and add a warning...
				feature.Enabled = false
				entitlements.Features[featureName] = feature
			case codersdk.EntitlementGracePeriod:
				entitlements.Warnings = append(entitlements.Warnings,
					fmt.Sprintf("%s is enabled but your license for this feature is expired.", niceName))
			default:
			}
		}
	}

	return entitlements, nil
}

const (
	CurrentVersion        = 3
	HeaderKeyID           = "kid"
	AccountTypeSalesforce = "salesforce"
	VersionClaim          = "version"
)

var (
	ValidMethods = []string{"EdDSA"}

	ErrInvalidVersion        = xerrors.New("license must be version 3")
	ErrMissingKeyID          = xerrors.Errorf("JOSE header must contain %s", HeaderKeyID)
	ErrMissingLicenseExpires = xerrors.New("license missing license_expires")
)

type Features struct {
	UserLimit      int64 `json:"user_limit"`
	AuditLog       int64 `json:"audit_log"`
	BrowserOnly    int64 `json:"browser_only"`
	SCIM           int64 `json:"scim"`
	WorkspaceQuota int64 `json:"workspace_quota"`
}

type Claims struct {
	jwt.RegisteredClaims
	// LicenseExpires is the end of the legit license term, and the start of the grace period, if
	// there is one.  The standard JWT claim "exp" (ExpiresAt in jwt.RegisteredClaims, above) is
	// the end of the grace period (identical to LicenseExpires if there is no grace period).
	// The reason we use the standard claim for the end of the grace period is that we want JWT
	// processing libraries to consider the token "valid" until then.
	LicenseExpires *jwt.NumericDate `json:"license_expires,omitempty"`
	AccountType    string           `json:"account_type,omitempty"`
	AccountID      string           `json:"account_id,omitempty"`
	Trial          bool             `json:"trial"`
	AllFeatures    bool             `json:"all_features"`
	Version        uint64           `json:"version"`
	Features       Features         `json:"features"`
}

// Parse consumes a license and returns the claims.
func Parse(l string, keys map[string]ed25519.PublicKey) (jwt.MapClaims, error) {
	tok, err := jwt.Parse(
		l,
		keyFunc(keys),
		jwt.WithValidMethods(ValidMethods),
	)
	if err != nil {
		return nil, err
	}
	if claims, ok := tok.Claims.(jwt.MapClaims); ok && tok.Valid {
		version, ok := claims[VersionClaim].(float64)
		if !ok {
			return nil, ErrInvalidVersion
		}
		if int64(version) != CurrentVersion {
			return nil, ErrInvalidVersion
		}
		return claims, nil
	}
	return nil, xerrors.New("unable to parse Claims")
}

// validateDBLicense validates a database.License record, and if valid, returns the claims.  If
// unparsable or invalid, it returns an error
func validateDBLicense(l database.License, keys map[string]ed25519.PublicKey) (*Claims, error) {
	tok, err := jwt.ParseWithClaims(
		l.JWT,
		&Claims{},
		keyFunc(keys),
		jwt.WithValidMethods(ValidMethods),
	)
	if err != nil {
		return nil, err
	}
	if claims, ok := tok.Claims.(*Claims); ok && tok.Valid {
		if claims.Version != uint64(CurrentVersion) {
			return nil, ErrInvalidVersion
		}
		if claims.LicenseExpires == nil {
			return nil, ErrMissingLicenseExpires
		}
		return claims, nil
	}
	return nil, xerrors.New("unable to parse Claims")
}

func keyFunc(keys map[string]ed25519.PublicKey) func(*jwt.Token) (interface{}, error) {
	return func(j *jwt.Token) (interface{}, error) {
		keyID, ok := j.Header[HeaderKeyID].(string)
		if !ok {
			return nil, ErrMissingKeyID
		}
		k, ok := keys[keyID]
		if !ok {
			return nil, xerrors.Errorf("no key with ID %s", keyID)
		}
		return k, nil
	}
}
