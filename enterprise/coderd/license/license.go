package license

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"math"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
)

const (
	featureManagedAgentLimitHard codersdk.FeatureName = "managed_agent_limit_hard"
	featureManagedAgentLimitSoft codersdk.FeatureName = "managed_agent_limit_soft"
)

var (
	// Mapping of license feature names to the SDK feature name.
	// This is used to map from multiple usage period features into a single SDK
	// feature.
	featureGrouping = map[codersdk.FeatureName]struct {
		// The parent feature.
		sdkFeature codersdk.FeatureName
		// Whether the value of the license feature is the soft limit or the hard
		// limit.
		isSoft bool
	}{
		// Map featureManagedAgentLimitHard and featureManagedAgentLimitSoft to
		// codersdk.FeatureManagedAgentLimit.
		featureManagedAgentLimitHard: {
			sdkFeature: codersdk.FeatureManagedAgentLimit,
			isSoft:     false,
		},
		featureManagedAgentLimitSoft: {
			sdkFeature: codersdk.FeatureManagedAgentLimit,
			isSoft:     true,
		},
	}

	// Features that are forbidden to be set in a license. These are the SDK
	// features in the usagedBasedFeatureGrouping map.
	licenseForbiddenFeatures = func() map[codersdk.FeatureName]struct{} {
		features := make(map[codersdk.FeatureName]struct{})
		for _, feature := range featureGrouping {
			features[feature.sdkFeature] = struct{}{}
		}
		return features
	}()
)

// Entitlements processes licenses to return whether features are enabled or not.
// TODO(@deansheather): This function and the related LicensesEntitlements
// function should be refactored into smaller functions that:
//  1. evaluate entitlements from fetched licenses
//  2. populate current usage values on the entitlements
//  3. generate warnings related to usage
func Entitlements(
	ctx context.Context,
	db database.Store,
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
	activeUserCount, err := db.GetActiveUserCount(dbauthz.AsSystemRestricted(ctx), false) // Don't include system user in license count.
	if err != nil {
		return codersdk.Entitlements{}, xerrors.Errorf("query active user count: %w", err)
	}

	// always shows active user count regardless of license
	entitlements, err := LicensesEntitlements(ctx, now, licenses, enablements, keys, FeatureArguments{
		ActiveUserCount:   activeUserCount,
		ReplicaCount:      replicaCount,
		ExternalAuthCount: externalAuthCount,
		ManagedAgentCountFn: func(ctx context.Context, from time.Time, to time.Time) (int64, error) {
			// TODO: this
			return 0, nil
		},
	})
	if err != nil {
		return entitlements, err
	}

	return entitlements, nil
}

type FeatureArguments struct {
	ActiveUserCount   int64
	ReplicaCount      int
	ExternalAuthCount int
	// Unfortunately, managed agent count is not a simple count of the current
	// state of the world, but a count between two points in time determined by
	// the licenses.
	ManagedAgentCountFn func(ctx context.Context, from time.Time, to time.Time) (int64, error)
}

// LicensesEntitlements returns the entitlements for licenses. Entitlements are
// merged from all licenses and the highest entitlement is used for each feature.
// Arguments:
//
//	now: The time to use for checking license expiration.
//	license: The license to check.
//	enablements: Features can be explicitly disabled by the deployment even if
//	             the license has the feature entitled. Features can also have
//	             the 'feat.AlwaysEnable()' return true to disallow disabling.
//	featureArguments: Additional arguments required by specific features.
func LicensesEntitlements(
	ctx context.Context,
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
		var vErr *jwt.ValidationError
		if xerrors.As(err, &vErr) && vErr.Is(jwt.ErrTokenNotValidYet) {
			// The license isn't valid yet.  We don't consider any entitlements contained in it, but
			// it's also not an error.  Just skip it silently.  This can happen if an administrator
			// uploads a license for a new term that hasn't started yet.
			continue
		}
		if err != nil {
			entitlements.Errors = append(entitlements.Errors,
				fmt.Sprintf("Invalid license (%s) parsing claims: %s", license.UUID.String(), err.Error()))
			continue
		}

		usagePeriodStart := claims.NotBefore.Time // checked not-nil when validating claims
		usagePeriodEnd := claims.ExpiresAt.Time   // checked not-nil when validating claims
		if usagePeriodStart.After(usagePeriodEnd) {
			// This shouldn't be possible to be hit. You'd need to have a
			// license with `nbf` after `exp`. Because `nbf` must be in the past
			// and `exp` must be in the future, this can never happen.
			entitlements.Errors = append(entitlements.Errors,
				fmt.Sprintf("Invalid license (%s): not_before (%s) is after license_expires (%s)", license.UUID.String(), usagePeriodStart, usagePeriodEnd))
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
			if _, ok := licenseForbiddenFeatures[featureName]; ok {
				// Ignore any FeatureSet features that are forbidden to be set
				// in a license.
				continue
			}
			if _, ok := featureGrouping[featureName]; ok {
				// These features need very special handling due to merging
				// multiple feature values into a single SDK feature.
				continue
			}
			if featureName == codersdk.FeatureUserLimit || featureName.UsesUsagePeriod() {
				// FeatureUserLimit and usage period features are handled below.
				// They don't provide default values as they are always enabled
				// and require a limit to be specified in the license to have
				// any effect.
				continue
			}

			entitlements.AddFeature(featureName, codersdk.Feature{
				Entitlement: entitlement,
				Enabled:     enablements[featureName] || featureName.AlwaysEnable(),
				Limit:       nil,
				Actual:      nil,
			})
		}

		// A map of SDK feature name to the uncommitted usage feature.
		uncommittedUsageFeatures := map[codersdk.FeatureName]usageLimit{}

		// Features al-la-carte
		for featureName, featureValue := range claims.Features {
			if _, ok := licenseForbiddenFeatures[featureName]; ok {
				entitlements.Errors = append(entitlements.Errors,
					fmt.Sprintf("Feature %s is forbidden to be set in a license.", featureName))
				continue
			}
			if featureValue < 0 {
				// We currently don't use negative values for features.
				continue
			}

			// Special handling for grouped (e.g. usage period) features.
			if grouping, ok := featureGrouping[featureName]; ok {
				ul := uncommittedUsageFeatures[grouping.sdkFeature]
				if grouping.isSoft {
					ul.Soft = &featureValue
				} else {
					ul.Hard = &featureValue
				}
				uncommittedUsageFeatures[grouping.sdkFeature] = ul
				continue
			}

			if _, ok := codersdk.FeatureNamesMap[featureName]; !ok {
				// Silently ignore any features that we don't know about.
				// They're either old features that no longer exist, or new
				// features that are not yet supported by the current server
				// version.
				continue
			}

			// Handling for non-grouped features.
			switch featureName {
			case codersdk.FeatureUserLimit:
				if featureValue <= 0 {
					// 0 user count doesn't make sense, so we skip it.
					continue
				}
				entitlements.AddFeature(codersdk.FeatureUserLimit, codersdk.Feature{
					Enabled:     true,
					Entitlement: entitlement,
					Limit:       &featureValue,
					Actual:      &featureArguments.ActiveUserCount,
				})

				// Temporary: If the license doesn't have a managed agent limit,
				//            we add a default of 800 managed agents per user.
				//            This only applies to "Premium" licenses.
				if claims.FeatureSet == codersdk.FeatureSetPremium {
					var (
						// We intentionally use a fixed issue time here, before the
						// entitlement was added to any new licenses, so any
						// licenses with the corresponding features actually set
						// trump this default entitlement, even if they are set to a
						// smaller value.
						issueTime             = time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC)
						defaultSoftAgentLimit = 800 * featureValue
						defaultHardAgentLimit = 1000 * featureValue
					)
					entitlements.AddFeature(codersdk.FeatureManagedAgentLimit, codersdk.Feature{
						Enabled:             true,
						Entitlement:         entitlement,
						SoftLimit:           &defaultSoftAgentLimit,
						Limit:               &defaultHardAgentLimit,
						UsagePeriodIssuedAt: &issueTime,
						UsagePeriodStart:    &usagePeriodStart,
						UsagePeriodEnd:      &usagePeriodEnd,
					})
				}
			default:
				if featureValue <= 0 {
					// The feature is disabled.
					continue
				}
				entitlements.Features[featureName] = codersdk.Feature{
					Entitlement: entitlement,
					Enabled:     enablements[featureName] || featureName.AlwaysEnable(),
				}
			}
		}

		// Apply uncommitted usage features to the entitlements.
		for featureName, ul := range uncommittedUsageFeatures {
			if ul.Soft == nil || ul.Hard == nil {
				// Invalid license.
				entitlements.Errors = append(entitlements.Errors,
					fmt.Sprintf("Invalid license (%s): feature %s has missing soft or hard limit values", license.UUID.String(), featureName))
				continue
			}
			if *ul.Hard < *ul.Soft {
				entitlements.Errors = append(entitlements.Errors,
					fmt.Sprintf("Invalid license (%s): feature %s has a hard limit less than the soft limit", license.UUID.String(), featureName))
				continue
			}
			if *ul.Hard < 0 || *ul.Soft < 0 {
				entitlements.Errors = append(entitlements.Errors,
					fmt.Sprintf("Invalid license (%s): feature %s has a soft or hard limit less than 0", license.UUID.String(), featureName))
				continue
			}

			feature := codersdk.Feature{
				Enabled:     true,
				Entitlement: entitlement,
				SoftLimit:   ul.Soft,
				Limit:       ul.Hard,
				// Actual value will be populated below when warnings are
				// generated.
				UsagePeriodIssuedAt: &claims.IssuedAt.Time,
				UsagePeriodStart:    &usagePeriodStart,
				UsagePeriodEnd:      &usagePeriodEnd,
			}
			// If the hard limit is 0, the feature is disabled.
			if *ul.Hard <= 0 {
				feature.Enabled = false
				feature.SoftLimit = ptr.Ref(int64(0))
				feature.Limit = ptr.Ref(int64(0))
			}
			entitlements.AddFeature(featureName, feature)
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

	// Managed agent warnings are applied based on usage period. We only
	// generate a warning if the license actually has managed agents.
	// Note that agents are free when unlicensed.
	agentLimit := entitlements.Features[codersdk.FeatureManagedAgentLimit]
	if entitlements.HasLicense && agentLimit.UsagePeriodStart != nil && agentLimit.UsagePeriodEnd != nil {
		// Calculate the amount of agents between the usage period start and
		// end.
		var (
			managedAgentCount int64
			err               = xerrors.New("dev error: managed agent count function is not set")
		)
		if featureArguments.ManagedAgentCountFn != nil {
			managedAgentCount, err = featureArguments.ManagedAgentCountFn(ctx, *agentLimit.UsagePeriodStart, *agentLimit.UsagePeriodEnd)
		}
		if err != nil {
			entitlements.Errors = append(entitlements.Errors,
				fmt.Sprintf("Error getting managed agent count: %s", err.Error()))
		} else {
			agentLimit.Actual = &managedAgentCount
			entitlements.AddFeature(codersdk.FeatureManagedAgentLimit, agentLimit)

			var softLimit int64
			if agentLimit.SoftLimit != nil {
				softLimit = *agentLimit.SoftLimit
			}
			var hardLimit int64
			if agentLimit.Limit != nil {
				hardLimit = *agentLimit.Limit
			}

			// Issue a warning early:
			// 1. If the soft limit and hard limit are equal, at 75% of the hard
			//    limit.
			// 2. If the limit is greater than the soft limit, at 75% of the
			//    difference between the hard limit and the soft limit.
			softWarningThreshold := int64(float64(hardLimit) * 0.75)
			if hardLimit > softLimit && softLimit > 0 {
				softWarningThreshold = softLimit + int64(float64(hardLimit-softLimit)*0.75)
			}
			if managedAgentCount >= *agentLimit.Limit {
				entitlements.Warnings = append(entitlements.Warnings,
					"You have built more workspaces with managed agents than your license allows. Further managed agent builds will be blocked.")
			} else if managedAgentCount >= softWarningThreshold {
				entitlements.Warnings = append(entitlements.Warnings,
					"You are approaching the managed agent limit in your license. Please refer to the Deployment Licenses page for more information.")
			}
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
			// Managed agent limits have it's own warnings based on the number of built agents!
			if featureName == codersdk.FeatureManagedAgentLimit {
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
	ErrMissingIssuedAt       = xerrors.New("license has invalid or missing iat (issued at) claim")
	ErrMissingNotBefore      = xerrors.New("license has invalid or missing nbf (not before) claim")
	ErrMissingLicenseExpires = xerrors.New("license has invalid or missing license_expires claim")
	ErrMissingExp            = xerrors.New("license has invalid or missing exp (expires at) claim")
	ErrMultipleIssues        = xerrors.New("license has multiple issues; contact support")
)

type Features map[codersdk.FeatureName]int64

type usageLimit struct {
	Soft *int64
	Hard *int64 // 0 means "disabled"
}

// Claims is the full set of claims in a license.
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

var _ jwt.Claims = &Claims{}

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

// ParseClaims validates a raw JWT, and if valid, returns the claims.  If
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
	return validateClaims(tok)
}

func validateClaims(tok *jwt.Token) (*Claims, error) {
	if claims, ok := tok.Claims.(*Claims); ok {
		if claims.Version != uint64(CurrentVersion) {
			return nil, ErrInvalidVersion
		}
		if claims.IssuedAt == nil {
			return nil, ErrMissingIssuedAt
		}
		if claims.NotBefore == nil {
			return nil, ErrMissingNotBefore
		}
		if claims.LicenseExpires == nil {
			return nil, ErrMissingLicenseExpires
		}
		if claims.ExpiresAt == nil {
			return nil, ErrMissingExp
		}
		return claims, nil
	}
	return nil, xerrors.New("unable to parse Claims")
}

// ParseClaimsIgnoreNbf validates a raw JWT, but ignores `nbf` claim. If otherwise valid, it returns
// the claims.  If unparsable or invalid, it returns an error. Ignoring the `nbf` (not before) is
// useful to determine if a JWT _will_ become valid at any point now or in the future.
func ParseClaimsIgnoreNbf(rawJWT string, keys map[string]ed25519.PublicKey) (*Claims, error) {
	tok, err := jwt.ParseWithClaims(
		rawJWT,
		&Claims{},
		keyFunc(keys),
		jwt.WithValidMethods(ValidMethods),
	)
	var vErr *jwt.ValidationError
	if xerrors.As(err, &vErr) {
		// zero out the NotValidYet error to check if there were other problems
		vErr.Errors &= (^jwt.ValidationErrorNotValidYet)
		if vErr.Errors != 0 {
			// There are other errors besides not being valid yet. We _could_ go
			// through all the jwt.ValidationError bits and try to work out the
			// correct error, but if we get here something very strange is
			// going on so let's just return a generic error that says to get in
			// touch with our support team.
			return nil, ErrMultipleIssues
		}
	} else if err != nil {
		return nil, err
	}
	return validateClaims(tok)
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
