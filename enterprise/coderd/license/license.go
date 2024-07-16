package license

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"math"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/codersdk"
)

// Entitlements processes licenses to return whether features are enabled or not.
func Entitlements(
	ctx context.Context,
	db database.Store,
	logger slog.Logger,
	replicaCount int,
	externalAuthCount int,
	keys map[string]ed25519.PublicKey,
	enablements map[codersdk.FeatureName]bool,
) (codersdk.Entitlements, error) {
	now := time.Now()

	// nolint:gocritic // Getting unexpired licenses is a system function.
	licenses, err := db.GetUnexpiredLicenses(dbauthz.AsSystemRestricted(ctx))
	if err != nil {
		return codersdk.Entitlements{}, err
	}

	// nolint:gocritic // Getting active user count is a system function.
	activeUserCount, err := db.GetActiveUserCount(dbauthz.AsSystemRestricted(ctx))
	if err != nil {
		return codersdk.Entitlements{}, xerrors.Errorf("query active user count: %w", err)
	}

	// always shows active user count regardless of license
	entitlements, err := LicensesEntitlements(now, licenses, enablements, keys, FeatureArguments{
		ActiveUserCount:   activeUserCount,
		ReplicaCount:      replicaCount,
		ExternalAuthCount: externalAuthCount,
	})

	return entitlements, nil
}

type FeatureArguments struct {
	ActiveUserCount   int64
	ReplicaCount      int
	ExternalAuthCount int
}

// LicensesEntitlements returns the entitlements for licenses. Entitlements are
// merged from all licenses and the highest entitlement is used for each feature.
// Arguments:
//
//	now: The time to use for checking license expiration.
//	license: The license to check.
//	enablements: Features can be explicitly disabled by the deployment even if
//				 the license has the feature entitled. Features can also have
//	             the 'feat.AlwaysEnable()' return true to disallow disabling.
//	featureArguments: Additional arguments required by specific features.
func LicensesEntitlements(
	now time.Time,
	licenses []database.License,
	enablements map[codersdk.FeatureName]bool,
	keys map[string]ed25519.PublicKey,
	featureArguments FeatureArguments,
) (codersdk.Entitlements, error) {

	// Default all entitlements to be disabled.
	entitlements := codersdk.Entitlements{
		Features: map[codersdk.FeatureName]codersdk.Feature{
			// always shows active user count regardless of license.
			codersdk.FeatureUserLimit: {
				Entitlement: codersdk.EntitlementNotEntitled,
				Enabled:     enablements[codersdk.FeatureUserLimit],
				Actual:      &featureArguments.ActiveUserCount,
			},
		},
		Warnings: []string{},
		Errors:   []string{},
	}

	// By default, enumerate all features and set them to not entitled.
	for _, featureName := range codersdk.FeatureNames {
		entitlements.AddFeature(featureName, codersdk.Feature{
			Entitlement: codersdk.EntitlementNotEntitled,
			Enabled:     enablements[featureName],
		})
	}

	// TODO: License specific warnings and errors should be tied to the license, not the
	//   'Entitlements' group as a whole.
	for _, license := range licenses {
		claims, err := ParseClaims(license.JWT, keys)
		if err != nil {
			entitlements.Errors = append(entitlements.Errors,
				fmt.Sprintf("Invalid license (%s) parsing claims: %s", license.UUID.String(), err.Error()))
			continue
		}

		// Any valid license should toggle this boolean
		entitlements.HasLicense = true

		// If any license requires telemetry, the deployment should require telemetry.
		entitlements.RequireTelemetry = entitlements.RequireTelemetry || claims.RequireTelemetry

		// entitlement is the highest entitlement for any features in this license.
		entitlement := codersdk.EntitlementEntitled
		// If any license is a trial license, this should be set to true.
		// The user should delete the trial license to remove this.
		entitlements.Trial = claims.Trial
		if now.After(claims.LicenseExpires.Time) {
			// if the grace period were over, the validation fails, so if we are after
			// LicenseExpires we must be in grace period.
			entitlement = codersdk.EntitlementGracePeriod
		}

		// Will add a warning if the license is expiring soon.
		// This warning can be raised multiple times if there is more than 1 license.
		licenseExpirationWarning(&entitlements, now, claims)

		// 'claims.AllFeature' is the legacy way to set 'claims.FeatureSet = codersdk.FeatureSetEnterprise'
		// If both are set, ignore the legacy 'claims.AllFeature'
		if claims.AllFeatures && claims.FeatureSet == "" {
			claims.FeatureSet = codersdk.FeatureSetEnterprise
		}

		// Add all features from the feature set defined.
		for _, featureName := range claims.FeatureSet.Features() {
			if featureName == codersdk.FeatureUserLimit {
				// FeatureUserLimit is unique in that it must be specifically defined
				// in the license. There is no default meaning if no "limit" is set.
				continue
			}
			entitlements.AddFeature(featureName, codersdk.Feature{
				Entitlement: entitlement,
				Enabled:     enablements[featureName] || featureName.AlwaysEnable(),
				Limit:       nil,
				Actual:      nil,
			})
		}

		// Features al-la-carte
		for featureName, featureValue := range claims.Features {
			// Can this be negative?
			if featureValue <= 0 {
				continue
			}

			switch featureName {
			case codersdk.FeatureUserLimit:
				// User limit has special treatment as our only non-boolean feature.
				limit := featureValue
				entitlements.AddFeature(codersdk.FeatureUserLimit, codersdk.Feature{
					Enabled:     true,
					Entitlement: entitlement,
					Limit:       &limit,
					Actual:      &featureArguments.ActiveUserCount,
				})
			default:
				entitlements.Features[featureName] = codersdk.Feature{
					Entitlement: entitlement,
					Enabled:     enablements[featureName] || featureName.AlwaysEnable(),
				}
			}
		}
	}

	// Now the license specific warnings and errors are added to the entitlements.

	// If HA is enabled, ensure the feature is entitled.
	if featureArguments.ReplicaCount > 1 {
		feature := entitlements.Features[codersdk.FeatureHighAvailability]

		switch feature.Entitlement {
		case codersdk.EntitlementNotEntitled:
			if entitlements.HasLicense {
				entitlements.Errors = append(entitlements.Errors,
					"You have multiple replicas but your license is not entitled to high availability. You will be unable to connect to workspaces.")
			} else {
				entitlements.Errors = append(entitlements.Errors,
					"You have multiple replicas but high availability is an Enterprise feature. You will be unable to connect to workspaces.")
			}
		case codersdk.EntitlementGracePeriod:
			entitlements.Warnings = append(entitlements.Warnings,
				"You have multiple replicas but your license for high availability is expired. Reduce to one replica or workspace connections will stop working.")
		}
	}

	if featureArguments.ExternalAuthCount > 1 {
		feature := entitlements.Features[codersdk.FeatureMultipleExternalAuth]

		switch feature.Entitlement {
		case codersdk.EntitlementNotEntitled:
			if entitlements.HasLicense {
				entitlements.Errors = append(entitlements.Errors,
					"You have multiple External Auth Providers configured but your license is limited at one.",
				)
			} else {
				entitlements.Errors = append(entitlements.Errors,
					"You have multiple External Auth Providers configured but this is an Enterprise feature. Reduce to one.",
				)
			}
		case codersdk.EntitlementGracePeriod:
			entitlements.Warnings = append(entitlements.Warnings,
				"You have multiple External Auth Providers configured but your license is expired. Reduce to one.",
			)
		}
	}

	if entitlements.HasLicense {
		userLimit := entitlements.Features[codersdk.FeatureUserLimit]
		if userLimit.Limit != nil && featureArguments.ActiveUserCount > *userLimit.Limit {
			entitlements.Warnings = append(entitlements.Warnings, fmt.Sprintf(
				"Your deployment has %d active users but is only licensed for %d.",
				featureArguments.ActiveUserCount, *userLimit.Limit))
		} else if userLimit.Limit != nil && userLimit.Entitlement == codersdk.EntitlementGracePeriod {
			entitlements.Warnings = append(entitlements.Warnings, fmt.Sprintf(
				"Your deployment has %d active users but the license with the limit %d is expired.",
				featureArguments.ActiveUserCount, *userLimit.Limit))
		}

		// Add a warning for every feature that is enabled but not entitled or
		// is in a grace period.
		for _, featureName := range codersdk.FeatureNames {
			// The user limit has it's own warnings!
			if featureName == codersdk.FeatureUserLimit {
				continue
			}
			// High availability has it's own warnings based on replica count!
			if featureName == codersdk.FeatureHighAvailability {
				continue
			}
			// External Auth Providers auth has it's own warnings based on the number configured!
			if featureName == codersdk.FeatureMultipleExternalAuth {
				continue
			}

			feature := entitlements.Features[featureName]
			if !feature.Enabled {
				continue
			}
			niceName := featureName.Humanize()
			switch feature.Entitlement {
			case codersdk.EntitlementNotEntitled:
				entitlements.Warnings = append(entitlements.Warnings,
					fmt.Sprintf("%s is enabled but your license is not entitled to this feature.", niceName))
			case codersdk.EntitlementGracePeriod:
				entitlements.Warnings = append(entitlements.Warnings,
					fmt.Sprintf("%s is enabled but your license for this feature is expired.", niceName))
			default:
			}
		}
	}

	// Wrap up by disabling all features that are not entitled.
	for _, featureName := range codersdk.FeatureNames {
		feature := entitlements.Features[featureName]
		if feature.Entitlement == codersdk.EntitlementNotEntitled {
			feature.Enabled = false
			entitlements.Features[featureName] = feature
		}
	}
	entitlements.RefreshedAt = now

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

type Features map[codersdk.FeatureName]int64

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
	// DeploymentIDs enforces the license can only be used on a set of deployments.
	DeploymentIDs []string            `json:"deployment_ids,omitempty"`
	Trial         bool                `json:"trial"`
	FeatureSet    codersdk.FeatureSet `json:"feature_set"`
	// AllFeatures represents 'FeatureSet = FeatureSetEnterprise'
	// Deprecated: AllFeatures is deprecated in favor of FeatureSet.
	AllFeatures      bool     `json:"all_features,omitempty"`
	Version          uint64   `json:"version"`
	Features         Features `json:"features"`
	RequireTelemetry bool     `json:"require_telemetry,omitempty"`
}

// ParseRaw consumes a license and returns the claims.
func ParseRaw(l string, keys map[string]ed25519.PublicKey) (jwt.MapClaims, error) {
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

// ParseClaims validates a database.License record, and if valid, returns the claims.  If
// unparsable or invalid, it returns an error
func ParseClaims(rawJWT string, keys map[string]ed25519.PublicKey) (*Claims, error) {
	tok, err := jwt.ParseWithClaims(
		rawJWT,
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

// maxEntitlement is the "greater" entitlement between the given values
func maxEntitlement(e1, e2 codersdk.Entitlement) codersdk.Entitlement {
	if e1 == codersdk.EntitlementEntitled || e2 == codersdk.EntitlementEntitled {
		return codersdk.EntitlementEntitled
	}
	if e1 == codersdk.EntitlementGracePeriod || e2 == codersdk.EntitlementGracePeriod {
		return codersdk.EntitlementGracePeriod
	}
	return codersdk.EntitlementNotEntitled
}

// licenseExpirationWarning adds a warning message if the license is expiring soon.
func licenseExpirationWarning(entitlements *codersdk.Entitlements, now time.Time, claims *Claims) {
	// Add warning if license is expiring soon
	daysToExpire := int(math.Ceil(claims.LicenseExpires.Sub(now).Hours() / 24))
	showWarningDays := 30
	isTrial := entitlements.Trial
	if isTrial {
		showWarningDays = 7
	}
	isExpiringSoon := daysToExpire > 0 && daysToExpire < showWarningDays
	if isExpiringSoon {
		day := "day"
		if daysToExpire > 1 {
			day = "days"
		}
		entitlements.Warnings = append(entitlements.Warnings, fmt.Sprintf("Your license expires in %d %s.", daysToExpire, day))
	}
}
