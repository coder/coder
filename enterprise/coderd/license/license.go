package license

import (
	"context"
	"crypto/ed25519"
	"database/sql"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
)

const (
	// These features are only included in the license and are not actually
	// entitlements after the licenses are processed. These values will be
	// merged into the codersdk.FeatureManagedAgentLimit feature.
	//
	// The reason we need two separate features is because the License v3 format
	// uses map[string]int64 for features, so we're unable to use a single value
	// with a struct like `{"soft": 100, "hard": 200}`. This is unfortunate and
	// we should fix this with a new license format v4 in the future.
	//
	// These are intentionally not exported as they should not be used outside
	// of this package (except tests).
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

	// nolint:gocritic // Getting external templates is a system function.
	externalTemplates, err := db.GetTemplatesWithFilter(dbauthz.AsSystemRestricted(ctx), database.GetTemplatesWithFilterParams{
		HasExternalAgent: sql.NullBool{
			Bool:  true,
			Valid: true,
		},
	})
	if err != nil {
		return codersdk.Entitlements{}, xerrors.Errorf("query external templates: %w", err)
	}

	entitlements, err := LicensesEntitlements(ctx, now, licenses, enablements, keys, FeatureArguments{
		ActiveUserCount:       activeUserCount,
		ReplicaCount:          replicaCount,
		ExternalAuthCount:     externalAuthCount,
		ExternalTemplateCount: int64(len(externalTemplates)),
		ManagedAgentCountFn: func(ctx context.Context, startTime time.Time, endTime time.Time) (int64, error) {
			// This is not super accurate, as the start and end times will be
			// truncated to the date in UTC timezone. This is an optimization
			// so we can use an aggregate table instead of scanning the usage
			// events table.
			//
			// High accuracy is not super necessary, as we give buffers in our
			// licenses (e.g. higher hard limit) to account for additional
			// usage.
			//
			// nolint:gocritic // Requires permission to read all workspaces to read managed agent count.
			return db.GetTotalUsageDCManagedAgentsV1(dbauthz.AsSystemRestricted(ctx), database.GetTotalUsageDCManagedAgentsV1Params{
				StartDate: startTime,
				EndDate:   endTime,
			})
		},
	})
	if err != nil {
		return entitlements, err
	}

	return entitlements, nil
}

type FeatureArguments struct {
	ActiveUserCount       int64
	ReplicaCount          int
	ExternalAuthCount     int
	ExternalTemplateCount int64
	// Unfortunately, managed agent count is not a simple count of the current
	// state of the world, but a count between two points in time determined by
	// the licenses.
	ManagedAgentCountFn ManagedAgentCountFn
}

type ManagedAgentCountFn func(ctx context.Context, from time.Time, to time.Time) (int64, error)

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

	// nextLicenseValidityPeriod holds the current or next contiguous period
	// where there will be at least one active license. This is used for
	// generating license expiry warnings. Previously we would generate licenses
	// expiry warnings for each license, but it means that the warning will show
	// even if you've loaded up a new license that doesn't have any gap.
	nextLicenseValidityPeriod := &licenseValidityPeriod{}

	// TODO: License specific warnings and errors should be tied to the license, not the
	//   'Entitlements' group as a whole.
	for _, license := range licenses {
		claims, err := ParseClaims(license.JWT, keys)
		var vErr *jwt.ValidationError
		if xerrors.As(err, &vErr) && vErr.Is(jwt.ErrTokenNotValidYet) {
			// The license isn't valid yet.  We don't consider any entitlements contained in it, but
			// it's also not an error.  Just skip it silently.  This can happen if an administrator
			// uploads a license for a new term that hasn't started yet.
			//
			// We still want to factor this into our validity period, though.
			// This ensures we can suppress license expiry warnings for expiring
			// licenses while a new license is ready to take its place.
			//
			// claims is nil, so reparse the claims with the IgnoreNbf function.
			claims, err = ParseClaimsIgnoreNbf(license.JWT, keys)
			if err != nil {
				continue
			}
			nextLicenseValidityPeriod.ApplyClaims(claims)
			continue
		}
		if err != nil {
			entitlements.Errors = append(entitlements.Errors,
				fmt.Sprintf("Invalid license (%s) parsing claims: %s", license.UUID.String(), err.Error()))
			continue
		}

		// Obviously, valid licenses should be considered for the license
		// validity period.
		nextLicenseValidityPeriod.ApplyClaims(claims)

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

		// 'claims.AllFeature' is the legacy way to set 'claims.FeatureSet = codersdk.FeatureSetEnterprise'
		// If both are set, ignore the legacy 'claims.AllFeature'
		if claims.AllFeatures && claims.FeatureSet == "" {
			claims.FeatureSet = codersdk.FeatureSetEnterprise
		}

		// Temporary: If the license doesn't have a managed agent limit, we add
		//            a default of 1000 managed agents per deployment for a 100
		//            year license term.
		//            This only applies to "Premium" licenses.
		if claims.FeatureSet == codersdk.FeatureSetPremium {
			var (
				// We intentionally use a fixed issue time here, before the
				// entitlement was added to any new licenses, so any
				// licenses with the corresponding features actually set
				// trump this default entitlement, even if they are set to a
				// smaller value.
				defaultManagedAgentsIsuedAt         = time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC)
				defaultManagedAgentsStart           = defaultManagedAgentsIsuedAt
				defaultManagedAgentsEnd             = defaultManagedAgentsStart.AddDate(100, 0, 0)
				defaultManagedAgentsSoftLimit int64 = 1000
				defaultManagedAgentsHardLimit int64 = 1000
			)
			entitlements.AddFeature(codersdk.FeatureManagedAgentLimit, codersdk.Feature{
				Enabled:     true,
				Entitlement: entitlement,
				SoftLimit:   &defaultManagedAgentsSoftLimit,
				Limit:       &defaultManagedAgentsHardLimit,
				UsagePeriod: &codersdk.UsagePeriod{
					IssuedAt: defaultManagedAgentsIsuedAt,
					Start:    defaultManagedAgentsStart,
					End:      defaultManagedAgentsEnd,
				},
			})
		}

		// Add all features from the feature set.
		for _, featureName := range claims.FeatureSet.Features() {
			if _, ok := licenseForbiddenFeatures[featureName]; ok {
				// Ignore any FeatureSet features that are forbidden to be set in a license.
				continue
			}
			if _, ok := featureGrouping[featureName]; ok {
				// These features need very special handling due to merging
				// multiple feature values into a single SDK feature.
				continue
			}
			if featureName.UsesLimit() || featureName.UsesUsagePeriod() {
				// Limit and usage period features are handled below.
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

			// Handling for limit features.
			switch {
			case featureName.UsesLimit():
				if featureValue <= 0 {
					// 0 user count doesn't make sense, so we skip it.
					continue
				}

				// When we have a limit feature, we need to set the actual value (if available).
				var actual *int64
				if featureName == codersdk.FeatureUserLimit {
					actual = &featureArguments.ActiveUserCount
				}

				entitlements.AddFeature(featureName, codersdk.Feature{
					Enabled:     true,
					Entitlement: entitlement,
					Limit:       &featureValue,
					Actual:      actual,
				})
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
				// `Actual` will be populated below when warnings are generated.
				UsagePeriod: &codersdk.UsagePeriod{
					IssuedAt: claims.IssuedAt.Time,
					Start:    usagePeriodStart,
					End:      usagePeriodEnd,
				},
			}
			// If the hard limit is 0, the feature is disabled.
			if *ul.Hard <= 0 {
				feature.Enabled = false
				feature.SoftLimit = ptr.Ref(int64(0))
				feature.Limit = ptr.Ref(int64(0))
			}
			entitlements.AddFeature(featureName, feature)
		}

		addonFeatures := make(map[codersdk.FeatureName]codersdk.Feature)

		// Add all features from the addons.
		for _, addon := range claims.Addons {
			validationErrors := addon.ValidateDependencies(entitlements.Features)
			if len(validationErrors) > 0 {
				entitlements.Errors = append(
					entitlements.Errors,
					validationErrors...,
				)
				// Ignore the addon and don't add any features.
				continue
			}
			for _, featureName := range addon.Features() {
				if _, exists := addonFeatures[featureName]; !exists {
					addonFeatures[featureName] = codersdk.Feature{
						Entitlement: entitlement,
						Enabled:     enablements[featureName] || featureName.AlwaysEnable(),
					}
				}
			}
		}
		for featureName, feature := range addonFeatures {
			entitlements.AddFeature(featureName, feature)
		}
	}

	// Now the license specific warnings and errors are added to the entitlements.

	// Add a single warning if we are currently in the license validity period
	// and it's expiring soon.
	nextLicenseValidityPeriod.LicenseExpirationWarning(&entitlements, now)

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

	if featureArguments.ExternalTemplateCount > 0 {
		feature := entitlements.Features[codersdk.FeatureWorkspaceExternalAgent]
		switch feature.Entitlement {
		case codersdk.EntitlementNotEntitled:
			entitlements.Errors = append(entitlements.Errors,
				"You have templates which use external agents but your license is not entitled to this feature.")
		case codersdk.EntitlementGracePeriod:
			entitlements.Warnings = append(entitlements.Warnings,
				"You have templates which use external agents but your license is expired.")
		}
	}

	// Managed agent warnings are applied based on usage period. We only
	// generate a warning if the license actually has managed agents.
	// Note that agents are free when unlicensed.
	agentLimit := entitlements.Features[codersdk.FeatureManagedAgentLimit]
	if entitlements.HasLicense && agentLimit.UsagePeriod != nil {
		// Calculate the amount of agents between the usage period start and
		// end.
		var (
			managedAgentCount int64
			err               = xerrors.New("dev error: managed agent count function is not set")
		)
		if featureArguments.ManagedAgentCountFn != nil {
			managedAgentCount, err = featureArguments.ManagedAgentCountFn(ctx, agentLimit.UsagePeriod.Start, agentLimit.UsagePeriod.End)
		}
		if xerrors.Is(err, context.Canceled) || xerrors.Is(err, context.DeadlineExceeded) {
			// If the context is canceled, we want to bail the entire
			// LicensesEntitlements call.
			return entitlements, xerrors.Errorf("get managed agent count: %w", err)
		}
		if err != nil {
			entitlements.Errors = append(entitlements.Errors, fmt.Sprintf("Error getting managed agent count: %s", err.Error()))
			// no return
		} else {
			agentLimit.Actual = &managedAgentCount
			entitlements.AddFeature(codersdk.FeatureManagedAgentLimit, agentLimit)

			// Only issue warnings if the feature is enabled.
			if agentLimit.Enabled {
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
	ErrMissingAccountType    = xerrors.New("license must contain valid account type")
	ErrMissingAccountID      = xerrors.New("license must contain valid account ID")
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
	AllFeatures      bool             `json:"all_features,omitempty"`
	Version          uint64           `json:"version"`
	Features         Features         `json:"features"`
	Addons           []codersdk.Addon `json:"addons,omitempty"`
	RequireTelemetry bool             `json:"require_telemetry,omitempty"`
	PublishUsageData bool             `json:"publish_usage_data,omitempty"`
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

		yearsHardLimit := time.Now().Add(5 /* years */ * 365 * 24 * time.Hour)
		if claims.LicenseExpires == nil || claims.LicenseExpires.Time.After(yearsHardLimit) {
			return nil, ErrMissingLicenseExpires
		}
		if claims.ExpiresAt == nil {
			return nil, ErrMissingExp
		}
		if claims.AccountType == "" {
			return nil, ErrMissingAccountType
		}
		if claims.AccountID == "" {
			return nil, ErrMissingAccountID
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

// licenseValidityPeriod keeps track of all license validity periods, and
// generates warnings over contiguous periods across multiple licenses.
//
// Note: this does not track the actual entitlements of each license to ensure
// newer licenses cover the same features as older licenses before merging. It
// is assumed that all licenses cover the same features.
type licenseValidityPeriod struct {
	// parts contains all tracked license periods prior to merging.
	parts [][2]time.Time
}

// ApplyClaims tracks a license validity period. This should only be called with
// valid (including not-yet-valid), unexpired licenses.
func (p *licenseValidityPeriod) ApplyClaims(claims *Claims) {
	if claims == nil || claims.NotBefore == nil || claims.LicenseExpires == nil {
		// Bad data
		return
	}
	p.Apply(claims.NotBefore.Time, claims.LicenseExpires.Time)
}

// Apply adds a license validity period.
func (p *licenseValidityPeriod) Apply(start, end time.Time) {
	if end.Before(start) {
		// Bad data
		return
	}
	p.parts = append(p.parts, [2]time.Time{start, end})
}

// merged merges the license validity periods into contiguous blocks, and sorts
// the merged blocks.
func (p *licenseValidityPeriod) merged() [][2]time.Time {
	if len(p.parts) == 0 {
		return nil
	}

	// Sort the input periods by start time.
	sorted := make([][2]time.Time, len(p.parts))
	copy(sorted, p.parts)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i][0].Before(sorted[j][0])
	})

	out := make([][2]time.Time, 0, len(sorted))
	cur := sorted[0]
	for i := 1; i < len(sorted); i++ {
		next := sorted[i]

		// If the current period's end time is before or equal to the next
		// period's start time, they should be merged.
		if !next[0].After(cur[1]) {
			// Pick the maximum end time.
			if next[1].After(cur[1]) {
				cur[1] = next[1]
			}
			continue
		}

		// They don't overlap, so commit the current period and start a new one.
		out = append(out, cur)
		cur = next
	}
	// Commit the final period.
	out = append(out, cur)
	return out
}

// LicenseExpirationWarning adds a warning message if we are currently in the
// license validity period and it's expiring soon.
func (p *licenseValidityPeriod) LicenseExpirationWarning(entitlements *codersdk.Entitlements, now time.Time) {
	merged := p.merged()
	if len(merged) == 0 {
		// No licenses
		return
	}
	end := merged[0][1]

	daysToExpire := int(math.Ceil(end.Sub(now).Hours() / 24))
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
