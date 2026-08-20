package license

import (
	"context"
	"crypto/ed25519"
	"database/sql"
	"fmt"
	"math"
	"slices"
	"sort"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
)

// Exceeding this timeout fails the entitlements computation; the caller
// keeps serving the previous entitlements. The count normally completes
// in well under a second, but its cost scales with the number of unique
// role sets and it runs on a context with no deadline of its own.
const workspaceCapableUserCountTimeout = 60 * time.Second

// Entitlements processes licenses to return whether features are enabled or not.
// TODO(@deansheather): This function and the related LicensesEntitlements
// function should be refactored into smaller functions that:
//  1. evaluate entitlements from fetched licenses
//  2. populate current usage values on the entitlements
//  3. generate warnings related to usage
func Entitlements(
	ctx context.Context,
	logger slog.Logger,
	db database.Store,
	replicaCount int,
	externalAuthCount int,
	keys map[string]ed25519.PublicKey,
	enablements map[codersdk.FeatureName]bool,
	authorizer rbac.Authorizer,
	experiments codersdk.Experiments,
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

	// Workspace-capable licensing counts only users the RBAC engine
	// authorizes to create workspaces. The mode alone decides whether the
	// counting function below is invoked.
	//
	// TODO: when the workspace-capable-licensing experiment is removed, a
	// nil authorizer must become a hard dev error rather than a silent
	// fallback to active-user counting. Tests already pass a real
	// authorizer; only the dedicated nil-fallback tests rely on this
	// branch.
	countingMode := UserCountingModeActive
	if experiments.Enabled(codersdk.ExperimentWorkspaceCapableLicensing) {
		if authorizer == nil {
			logger.Warn(ctx, "workspace-capable licensing experiment is enabled but no authorizer is configured, counting all active users")
		} else {
			countingMode = UserCountingModeWorkspaceCapable
		}
	}

	// nolint:gocritic // Getting active AI seat count is a system function.
	activeAISeatCount, err := db.GetActiveAISeatCount(dbauthz.AsSystemRestricted(ctx))
	if err != nil {
		return codersdk.Entitlements{}, xerrors.Errorf("query active AI seat count: %w", err)
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
		Logger:                logger,
		ActiveUserCount:       activeUserCount,
		ActiveAISeatCount:     activeAISeatCount,
		ReplicaCount:          replicaCount,
		ExternalAuthCount:     externalAuthCount,
		ExternalTemplateCount: int64(len(externalTemplates)),
		UserCountingMode:      countingMode,
		WorkspaceCapableUserCountFn: func(ctx context.Context) (int64, error) {
			ctx, cancel := context.WithTimeout(ctx, workspaceCapableUserCountTimeout)
			defer cancel()
			return CountWorkspaceCapableUsers(ctx, logger, db, authorizer)
		},
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
		AgentRuntimeMsFn: func(ctx context.Context, startTime time.Time, endTime time.Time) (int64, error) {
			// Bounds and bucket semantics are documented on the query.
			//
			// nolint:gocritic // Reading usage events requires the usage publisher subject.
			return db.GetTotalUsageHBAgentRuntimeV1(dbauthz.AsUsagePublisher(ctx), database.GetTotalUsageHBAgentRuntimeV1Params{
				StartTime: startTime,
				EndTime:   endTime,
			})
		},
	})
	if err != nil {
		return entitlements, err
	}

	return entitlements, nil
}

type FeatureArguments struct {
	Logger                slog.Logger
	ActiveUserCount       int64
	ActiveAISeatCount     int64
	ReplicaCount          int
	ExternalAuthCount     int
	ExternalTemplateCount int64
	// Unfortunately, managed agent count is not a simple count of the current
	// state of the world, but a count between two points in time determined by
	// the licenses.
	ManagedAgentCountFn ManagedAgentCountFn
	// AgentRuntimeMsFn is queried with two points in time determined by the
	// licenses, like the managed agent count above.
	AgentRuntimeMsFn AgentRuntimeMsFn
	// UserCountingMode selects the count that FeatureUserLimit candidates
	// from AI Governance addon licenses are evaluated against. Under
	// UserCountingModeWorkspaceCapable they use WorkspaceCapableUserCountFn's
	// count; under any other value, including the zero value, every
	// candidate uses ActiveUserCount.
	UserCountingMode UserCountingMode
	// WorkspaceCapableUserCountFn returns the number of active users the
	// RBAC engine authorizes to create workspaces. It is invoked only
	// under UserCountingModeWorkspaceCapable, and only when a valid
	// license carries both the AI Governance addon and a FeatureUserLimit
	// claim; the result then applies to that license's FeatureUserLimit
	// candidate, and replaces ActiveUserCount when such a candidate is
	// selected for enforcement. May be nil under UserCountingModeActive;
	// leaving it nil when the workspace-capable mode would invoke it is a
	// dev error.
	WorkspaceCapableUserCountFn WorkspaceCapableUserCountFn
}

// UserCountingMode selects how license seats are counted for
// FeatureUserLimit candidates from AI Governance addon licenses.
type UserCountingMode string

const (
	// UserCountingModeActive evaluates every FeatureUserLimit candidate
	// against the active user count.
	UserCountingModeActive UserCountingMode = "active_users"
	// UserCountingModeWorkspaceCapable evaluates addon-carrying candidates
	// against the workspace-capable user count.
	UserCountingModeWorkspaceCapable UserCountingMode = "workspace_capable_users"
)

type ManagedAgentCountFn func(ctx context.Context, from time.Time, to time.Time) (int64, error)

// AgentRuntimeMsFn returns the total Coder Agent runtime in milliseconds
// recorded between from (inclusive) and to (exclusive).
type AgentRuntimeMsFn = ManagedAgentCountFn

type WorkspaceCapableUserCountFn func(ctx context.Context) (int64, error)

// userLimitCandidate is one license's FeatureUserLimit terms: its seat limit,
// its entitlement, and the counting mode implied by whether the license
// carries the AI Governance addon (workspace-capable counting of
// workspace-capable users vs. counting all active users).
type userLimitCandidate struct {
	limit             int64
	entitlement       codersdk.Entitlement
	aiGovernanceAddon bool
}

// resolvedCandidate pairs a candidate with the count its counting mode
// implies: the workspace-capable count for addon candidates when
// workspace-capable counting is active, the active user count otherwise.
type resolvedCandidate struct {
	userLimitCandidate
	count int64
}

// betterUserLimit reports whether candidate a is more favorable than b.
// Ordering mirrors Feature.Compare: a candidate whose count is within its
// limit beats one whose count is not, then higher entitlement, then
// higher limit; the addon mode breaks remaining ties since its count is
// never larger than the active user count.
func betterUserLimit(a, b resolvedCandidate) bool {
	compliantA := a.count <= a.limit
	compliantB := b.count <= b.limit
	if compliantA != compliantB {
		return compliantA
	}
	if a.entitlement.Weight() != b.entitlement.Weight() {
		return a.entitlement.Weight() > b.entitlement.Weight()
	}
	if a.limit != b.limit {
		return a.limit > b.limit
	}
	return a.aiGovernanceAddon && !b.aiGovernanceAddon
}

// userLimitSelection reports how the enforced FeatureUserLimit was chosen.
type userLimitSelection struct {
	// workspaceCapable is true when the selected candidate counts
	// workspace-capable users rather than all active users.
	workspaceCapable bool
	// addonEntitled is true when at least one addon-carrying candidate is
	// fully valid rather than in its grace period.
	addonEntitled bool
}

// selectUserLimit picks the most favorable FeatureUserLimit candidate and
// applies its terms to the entitlements. Every candidate is evaluated
// against the count its own license's mode implies (the workspace-capable
// count for workspace-capable candidates, the active user count
// otherwise), so one license's limit is never combined with another
// license's counting mode. A candidate satisfied by its count wins over
// any unsatisfied one.
//
// For example, a deployment holding a 200-seat non-addon license and a
// 100-seat AI Governance license:
//
//	active | capable | 200-seat license | 100-seat addon license | selected
//	   250 |      90 | over             | satisfied              | addon:     90/100
//	   180 |     150 | satisfied        | over                   | non-addon: 180/200
//
// Neither license's limit is ever paired with the other's count: 90
// capable users against the 200-seat limit, or 180 active users against
// the 100-seat limit, are not considered.
//
// With no candidates the entitlements are left untouched. On a count
// failure the entitlements computation must be aborted.
func selectUserLimit(
	ctx context.Context,
	entitlements *codersdk.Entitlements,
	featureArguments FeatureArguments,
	candidates []userLimitCandidate,
) (userLimitSelection, error) {
	var sel userLimitSelection
	if len(candidates) == 0 {
		return sel, nil
	}

	hasAddonCandidate := false
	for _, c := range candidates {
		if c.aiGovernanceAddon {
			hasAddonCandidate = true
			if c.entitlement == codersdk.EntitlementEntitled {
				sel.addonEntitled = true
			}
		}
	}

	var capableCount *int64
	if hasAddonCandidate && featureArguments.UserCountingMode == UserCountingModeWorkspaceCapable {
		if featureArguments.WorkspaceCapableUserCountFn == nil {
			return sel, xerrors.New("dev error: workspace-capable user count function is not set")
		}
		count, err := featureArguments.WorkspaceCapableUserCountFn(ctx)
		if err != nil {
			// A failed seat count is deliberately a hard failure rather
			// than a recorded entitlement error: continuing with
			// ActiveUserCount would silently change what FeatureUserLimit
			// measures. The caller keeps the previous entitlements, so a
			// failure yields a stale count rather than a different one.
			return sel, xerrors.Errorf("count workspace capable users: %w", err)
		}
		capableCount = &count
	}

	resolved := make([]resolvedCandidate, len(candidates))
	for i, c := range candidates {
		resolved[i] = resolvedCandidate{userLimitCandidate: c, count: featureArguments.ActiveUserCount}
		if c.aiGovernanceAddon && capableCount != nil {
			resolved[i].count = *capableCount
		}
	}

	best := resolved[0]
	for _, c := range resolved[1:] {
		if betterUserLimit(c, best) {
			best = c
		}
	}

	if best.aiGovernanceAddon && capableCount != nil {
		sel.workspaceCapable = true
	}

	// AddFeature merged limits and entitlements across licenses without
	// pairing them to counting modes; overwrite the merged terms with the
	// selected candidate's. Actual is replaced wholesale, so the merged
	// feature's alias of the caller's ActiveUserCount no longer matters.
	userLimit := entitlements.Features[codersdk.FeatureUserLimit]
	userLimit.Limit = &best.limit
	userLimit.Entitlement = best.entitlement
	userLimit.Actual = &best.count
	entitlements.Features[codersdk.FeatureUserLimit] = userLimit
	return sel, nil
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
	// Each valid license's FeatureUserLimit claim forms a candidate pairing of
	// seat limit and counting mode: licenses carrying the AI Governance
	// addon count workspace-capable users, others count all active users.
	// The most favorable candidate is selected once all licenses are
	// processed.
	var userLimitCandidates []userLimitCandidate

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
				defaultManagedAgentsIsuedAt       = time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC)
				defaultManagedAgentsStart         = defaultManagedAgentsIsuedAt
				defaultManagedAgentsEnd           = defaultManagedAgentsStart.AddDate(100, 0, 0)
				defaultManagedAgentsLimit   int64 = 1000
			)
			entitlements.AddFeature(codersdk.FeatureManagedAgentLimit, codersdk.Feature{
				Enabled:     true,
				Entitlement: entitlement,
				Limit:       &defaultManagedAgentsLimit,
				UsagePeriod: &codersdk.UsagePeriod{
					IssuedAt: defaultManagedAgentsIsuedAt,
					Start:    defaultManagedAgentsStart,
					End:      defaultManagedAgentsEnd,
				},
			})

			// Premium licenses without agent_runtime_hours_* claims are
			// grandfathered into a zero-hour allocation: the feature is
			// granted disabled with a zero limit, which measures and
			// publishes usage (see the measureAgentRuntimeMs call below)
			// and caps concurrent agentic chats the same as an explicit
			// zero allocation.
			var (
				// A fixed issue time that predates any license issued with
				// agent_runtime_hours_* claims, so a license that actually
				// carries those claims outranks this default in
				// Feature.Compare (IssuedAt-first for usage period features)
				// regardless of the licenses' relative issue dates. This
				// must remain earlier than the earliest legitimately issued
				// claim-bearing license.
				defaultAgentRuntimeHoursIssuedAt = time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
				defaultAgentRuntimeHoursLimit    int64
			)
			entitlements.AddFeature(codersdk.FeatureAgentRuntimeHours, codersdk.Feature{
				Enabled:     false,
				Entitlement: entitlement,
				Limit:       &defaultAgentRuntimeHoursLimit,
				UsagePeriod: &codersdk.UsagePeriod{
					IssuedAt: defaultAgentRuntimeHoursIssuedAt,
					// The license term, matching a license with an explicit
					// zero allocation, so measured usage covers the current
					// term.
					Start: usagePeriodStart,
					End:   usagePeriodEnd,
				},
			})
		}

		// Add all features from the feature set.
		for _, featureName := range claims.FeatureSet.Features() {
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

		// Features al-la-carte
		for featureName, featureValue := range claims.Features {
			// Old-style licenses encode the managed agent limit as
			// separate soft/hard features.
			//
			// This could be removed in a future release, but can only be
			// done once all old licenses containing this are no longer in use.
			if featureName == "managed_agent_limit_soft" {
				// Maps the soft limit to the canonical feature name
				featureName = codersdk.FeatureManagedAgentLimit
			}
			if featureName == "managed_agent_limit_hard" {
				// We can safely ignore the hard limit as it is no longer used.
				continue
			}

			// Agent runtime hour claims are decoded together after this
			// loop; see decodeAgentRuntimeHours.
			if featureName == codersdk.FeatureAgentRuntimeHours ||
				isAgentRuntimeHoursClaim(featureName) {
				continue
			}

			if featureValue < 0 {
				// We currently don't use negative values for features.
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
			case featureName.UsesUsagePeriod():
				entitlements.AddFeature(featureName, codersdk.Feature{
					Enabled:     featureValue > 0,
					Entitlement: entitlement,
					Limit:       &featureValue,
					UsagePeriod: &codersdk.UsagePeriod{
						IssuedAt: claims.IssuedAt.Time,
						Start:    usagePeriodStart,
						End:      usagePeriodEnd,
					},
				})
			case featureName.UsesLimit():
				if featureValue <= 0 {
					// 0 limit value or less doesn't make sense, so we skip it.
					continue
				}

				// When we have a limit feature, we need to set the actual value (if available).
				var actual *int64
				if featureName == codersdk.FeatureUserLimit {
					actual = &featureArguments.ActiveUserCount
				}
				if featureName == codersdk.FeatureAIGovernanceUserLimit {
					actual = &featureArguments.ActiveAISeatCount
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

		runtimeFeature, granted, ignoredClaims := decodeAgentRuntimeHours(claims.Features, entitlement, codersdk.UsagePeriod{
			IssuedAt: claims.IssuedAt.Time,
			Start:    usagePeriodStart,
			End:      usagePeriodEnd,
		})
		if granted {
			entitlements.AddFeature(codersdk.FeatureAgentRuntimeHours, runtimeFeature)
		}
		if len(ignoredClaims) > 0 {
			featureArguments.Logger.Warn(ctx, "ignored unusable Coder Agent runtime hour claims in license",
				slog.F("license_id", license.UUID),
				slog.F("ignored_claims", ignoredClaims),
			)
			if !slices.Contains(entitlements.Warnings, codersdk.LicenseAgentRuntimeHoursClaimsIgnoredWarningText) {
				entitlements.Warnings = append(entitlements.Warnings,
					codersdk.LicenseAgentRuntimeHoursClaimsIgnoredWarningText)
			}
		}

		addonFeatures := make(map[codersdk.FeatureName]codersdk.Feature)
		licenseHasAIGovernanceAddon := false

		// Finally, add all features from the addons. We do this last so that
		// any dependencies of an addon are validated against the calculated
		// found entitlements. This is to stop a race condition with how we
		// calculate entitlements in tests.
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
			if addon == codersdk.AddonAIGovernance {
				licenseHasAIGovernanceAddon = true
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

		if limit := claims.Features[codersdk.FeatureUserLimit]; limit > 0 {
			userLimitCandidates = append(userLimitCandidates, userLimitCandidate{
				limit:             limit,
				entitlement:       entitlement,
				aiGovernanceAddon: licenseHasAIGovernanceAddon,
			})
		}
	}

	// The FeatureUserLimit feature's final terms come from best-pair selection
	// across the candidates rather than the AddFeature merge.
	userLimitSel, err := selectUserLimit(ctx, &entitlements, featureArguments, userLimitCandidates)
	if err != nil {
		return entitlements, err
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
			if agentLimit.Enabled && agentLimit.Limit != nil && managedAgentCount >= *agentLimit.Limit {
				entitlements.Warnings = append(entitlements.Warnings,
					codersdk.LicenseManagedAgentLimitExceededWarningText)
			}
		}
	}

	// Usage is measured even for a zero allocation, which reports the
	// feature disabled: see decodeAgentRuntimeHours. Premium licenses
	// without agent runtime hour claims grant the same disabled zero-limit
	// feature (see the grandfather default above), so every premium
	// deployment reports usage here. Reported usage can trail real usage;
	// the sources of staleness and loss are documented on the
	// enterprise/coderd/usage.AgentRuntime* constants.
	runtimeHours := entitlements.Features[codersdk.FeatureAgentRuntimeHours]
	if entitlements.HasLicense && runtimeHours.UsagePeriod != nil {
		runtimeMs, ok, err := measureAgentRuntimeMs(ctx, &entitlements,
			featureArguments.Logger, featureArguments.AgentRuntimeMsFn, *runtimeHours.UsagePeriod)
		if err != nil {
			return entitlements, err
		}
		if ok {
			actualHours := agentRuntimeMsToHours(runtimeMs)
			runtimeHours.Actual = &actualHours
			// ActualMs carries the exact stored milliseconds so clients can
			// render fractional hours. Negative input clamps to 0, mirroring
			// agentRuntimeMsToHours, since AgentRuntimeMsFn is a
			// caller-supplied seam.
			actualMs := max(runtimeMs, 0)
			runtimeHours.ActualMs = &actualMs
			// Written back directly rather than through AddFeature:
			// AddFeature only replaces the existing entry when the new one
			// strictly outranks it, so setting Actual on an otherwise
			// identical feature would be dropped as a tie.
			entitlements.Features[codersdk.FeatureAgentRuntimeHours] = runtimeHours

			// A nil Limit means the license grants unlimited runtime
			// hours: no thresholds can exist, so no warnings.
			if runtimeHours.Limit != nil {
				entitlements.Warnings = appendAgentRuntimeHoursWarning(
					entitlements.Warnings, actualHours, *runtimeHours.Limit, runtimeHours.SoftLimit)
			}
		}
	}

	if entitlements.HasLicense {
		userLimit := entitlements.Features[codersdk.FeatureUserLimit]
		// The enforced count and its meaning come from the selected
		// candidate: userLimit.Actual is the count the limit was evaluated
		// against, and the noun names what it counted.
		userLimitActual := featureArguments.ActiveUserCount
		if userLimit.Actual != nil {
			userLimitActual = *userLimit.Actual
		}
		userNoun := "active users"
		if userLimitSel.workspaceCapable {
			userNoun = "workspace-capable users"
		}
		if userLimit.Limit != nil && userLimitActual > *userLimit.Limit {
			entitlements.Warnings = append(entitlements.Warnings, fmt.Sprintf(
				"Your deployment has %d %s but is only licensed for %d.",
				userLimitActual, userNoun, *userLimit.Limit))
		} else if userLimit.Limit != nil && userLimit.Entitlement == codersdk.EntitlementGracePeriod {
			entitlements.Warnings = append(entitlements.Warnings, fmt.Sprintf(
				"Your deployment has %d %s but the license with the limit %d is expired.",
				userLimitActual, userNoun, *userLimit.Limit))
		}
		// The addon exists only on grace-period licenses: warn that
		// workspace-capable counting stops at the end of the grace period,
		// at which point every active user counts.
		if userLimitSel.workspaceCapable && !userLimitSel.addonEntitled {
			entitlements.Warnings = append(entitlements.Warnings, fmt.Sprintf(
				"Your license with the AI Governance addon is expired. When it fully expires, all %d active users will count toward the user limit instead of the %d workspace-capable users.",
				featureArguments.ActiveUserCount, userLimitActual))
		}
		if featureArguments.ActiveAISeatCount > 0 {
			actual := featureArguments.ActiveAISeatCount
			feature := entitlements.Features[codersdk.FeatureAIGovernanceUserLimit]
			switch {
			case feature.Entitlement == codersdk.EntitlementNotEntitled:
				// Not-entitled deployments can accumulate phantom ai_seat_state
				// rows from prior Gateway testing or Task usage. Surfacing an
				// error here is alarming and inactionable for customers who
				// never purchased the AI Governance addon.
			case feature.Entitlement == codersdk.EntitlementGracePeriod && feature.Limit != nil:
				entitlements.Warnings = append(entitlements.Warnings,
					fmt.Sprintf(
						"Your deployment has %d active AI Governance seats but the license with the limit %d is expired.",
						actual, *feature.Limit))
				// Also emit seat-capacity warnings during grace period so admins
				// see both expiry and usage details.
				entitlements.Warnings = appendAIGovernanceSeatLimitWarning(
					entitlements.Warnings,
					actual,
					*feature.Limit,
				)
			case feature.Limit != nil:
				entitlements.Warnings = appendAIGovernanceSeatLimitWarning(
					entitlements.Warnings,
					actual,
					*feature.Limit,
				)
			}
		}

		// Add a warning for every feature that is enabled but not entitled or
		// is in a grace period.
		for _, featureName := range codersdk.FeatureNames {
			// The user limit has it's own warnings!
			if featureName == codersdk.FeatureUserLimit {
				continue
			}
			if featureName == codersdk.FeatureAIGovernanceUserLimit {
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
			// Agent runtime hours is a usage period feature and does not
			// generate generic entitlement warnings.
			if featureName == codersdk.FeatureAgentRuntimeHours {
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

// measureAgentRuntimeMs runs fn over the feature's usage period. A nil fn
// or a failure with a dead context fails the whole call; any other failure
// logs the cause and publishes the stable unavailable text instead. It
// returns the measured milliseconds and true only on success.
func measureAgentRuntimeMs(
	ctx context.Context,
	entitlements *codersdk.Entitlements,
	logger slog.Logger,
	fn AgentRuntimeMsFn,
	usagePeriod codersdk.UsagePeriod,
) (int64, bool, error) {
	if fn == nil {
		return 0, false, xerrors.New("developer error: no closure provided to measure agent runtime usage")
	}
	value, err := fn(ctx, usagePeriod.Start, usagePeriod.End)
	switch {
	case err != nil && ctx.Err() != nil:
		// Do not classify cancellation by error shape instead of ctx.Err():
		// Postgres raises SQLSTATE 57014 (query_canceled) for
		// statement_timeout kills as well as client cancels, and aborting on
		// those would fail every entitlements refresh on a deployment whose
		// statement_timeout is shorter than a usage query.
		return 0, false, xerrors.Errorf("get agent runtime: %w", err)
	case err != nil:
		logger.Error(ctx, "get agent runtime for entitlements", slog.Error(err))
		entitlements.Errors = append(entitlements.Errors, codersdk.LicenseAgentRuntimeUsageUnavailableErrorText)
		return 0, false, nil
	}
	return value, true, nil
}

// appendAgentRuntimeHoursWarning appends at most one warning: reaching the
// allocation supersedes the advisory soft limit, so the dashboard banner
// never stacks both messages.
func appendAgentRuntimeHoursWarning(warnings []string, actualHours int64, allocation int64, softLimit *int64) []string {
	// A zero allocation (explicit or the grandfathered premium default) has
	// no thresholds to warn about: those deployments are steered by the
	// in-page upgrade CTA and the concurrent chat cap, not a
	// deployment-wide banner.
	if allocation <= 0 {
		return warnings
	}

	switch {
	case actualHours >= allocation:
		return append(warnings, fmt.Sprintf(
			codersdk.LicenseAgentRuntimeHoursAllocationReachedWarningText,
			actualHours, allocation))
	case softLimit != nil && actualHours >= *softLimit:
		return append(warnings, fmt.Sprintf(
			codersdk.LicenseAgentRuntimeHoursSoftLimitWarningText,
			actualHours, allocation, *softLimit))
	}

	return warnings
}

func appendAIGovernanceSeatLimitWarning(warnings []string, actual int64, limit int64) []string {
	if limit <= 0 {
		return warnings
	}

	if actual > limit {
		overLimitSeats := actual - limit
		return append(warnings, fmt.Sprintf(
			codersdk.LicenseAIGovernanceOverLimitWarningText,
			actual,
			limit,
			overLimitSeats,
		))
	} else if actual*10 >= limit*9 {
		usedPercent := (actual * 100) / limit
		return append(warnings, fmt.Sprintf(codersdk.LicenseAIGovernance90PercentWarningText, usedPercent))
	}

	return warnings
}

const (
	CurrentVersion        = 3
	HeaderKeyID           = "kid"
	AccountTypeSalesforce = "salesforce"
	VersionClaim          = "version"
)

// Agent runtime hour license claims, minted by github.com/coder/license.
// All three are in hours and decode together into the single
// codersdk.FeatureAgentRuntimeHours feature; see decodeAgentRuntimeHours.
const (
	// ClaimAgentRuntimeHoursAllocation is the purchased runtime-hour
	// allocation for the license term. It becomes the feature's Limit.
	// AgentRuntimeHoursUnlimitedAllocation (-1) is reserved to mean
	// unlimited; any other negative allocation is ignored, in which case
	// the license does not grant the feature.
	ClaimAgentRuntimeHoursAllocation = "agent_runtime_hours_allocation"
	// ClaimAgentRuntimeHoursLimitSoft is the advisory warning threshold. It
	// becomes the feature's SoftLimit when 0 <= soft < allocation and is
	// ignored otherwise.
	ClaimAgentRuntimeHoursLimitSoft = "agent_runtime_hours_limit_soft"
	// ClaimAgentRuntimeHoursLimitHard is the enforcement ceiling. It becomes
	// the feature's HardLimit when the allocation is greater than 0 and
	// hard >= allocation, and is ignored otherwise.
	ClaimAgentRuntimeHoursLimitHard = "agent_runtime_hours_limit_hard"
)

// AgentRuntimeHoursUnlimitedAllocation is the reserved
// ClaimAgentRuntimeHoursAllocation value meaning the license grants
// unlimited runtime hours. It decodes to an enabled feature with a nil
// Limit. Mirrored in github.com/coder/license.
const AgentRuntimeHoursUnlimitedAllocation int64 = -1

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

// isAgentRuntimeHoursClaim reports whether name is one of the three claims
// decoded by decodeAgentRuntimeHours.
func isAgentRuntimeHoursClaim(name codersdk.FeatureName) bool {
	switch name {
	case ClaimAgentRuntimeHoursAllocation,
		ClaimAgentRuntimeHoursLimitSoft,
		ClaimAgentRuntimeHoursLimitHard:
		return true
	default:
		return false
	}
}

// agentRuntimeMsToHours floors milliseconds of Coder Agent runtime to whole
// hours, the unit shared by the agent_runtime_hours_* claims and the
// feature's limits. Flooring keeps the rendered value and the whole-hour
// warning thresholds in agreement. Negative input (not producible by the
// production query, but AgentRuntimeMsFn is a caller-supplied seam) clamps
// to 0.
func agentRuntimeMsToHours(ms int64) int64 {
	if ms <= 0 {
		return 0
	}
	return ms / int64(time.Hour/time.Millisecond)
}

// decodeAgentRuntimeHours builds the codersdk.FeatureAgentRuntimeHours
// feature from its claims. granted is false when there is no usable
// allocation claim; per-claim validity rules live on the Claim* constants
// above.
//
// Unusable claims are dropped rather than invalidating the license, since
// rejecting a signed license over a cosmetic claim would drop the deployment
// to unlicensed. Each dropped claim is returned in ignoredClaims so the
// caller can warn and log instead of letting an incorrectly issued license
// look healthy.
//
// A zero allocation grants the feature disabled, but Actual is still
// measured and published.
func decodeAgentRuntimeHours(features Features, entitlement codersdk.Entitlement, usagePeriod codersdk.UsagePeriod) (feature codersdk.Feature, granted bool, ignoredClaims []string) {
	if _, ok := features[codersdk.FeatureAgentRuntimeHours]; ok {
		ignoredClaims = append(ignoredClaims, string(codersdk.FeatureAgentRuntimeHours))
	}

	allocation, allocOk := features[ClaimAgentRuntimeHoursAllocation]
	soft, softOk := features[ClaimAgentRuntimeHoursLimitSoft]
	hard, hardOk := features[ClaimAgentRuntimeHoursLimitHard]

	if allocOk && allocation == AgentRuntimeHoursUnlimitedAllocation {
		if softOk {
			ignoredClaims = append(ignoredClaims, ClaimAgentRuntimeHoursLimitSoft)
		}
		if hardOk {
			ignoredClaims = append(ignoredClaims, ClaimAgentRuntimeHoursLimitHard)
		}
		return codersdk.Feature{
			Enabled:     true,
			Entitlement: entitlement,
			UsagePeriod: &usagePeriod,
		}, true, ignoredClaims
	}

	if !allocOk || allocation < 0 {
		if allocOk && allocation < 0 {
			ignoredClaims = append(ignoredClaims, ClaimAgentRuntimeHoursAllocation)
		}
		if softOk {
			ignoredClaims = append(ignoredClaims, ClaimAgentRuntimeHoursLimitSoft)
		}
		if hardOk {
			ignoredClaims = append(ignoredClaims, ClaimAgentRuntimeHoursLimitHard)
		}
		return codersdk.Feature{}, false, ignoredClaims
	}

	feature = codersdk.Feature{
		Enabled:     allocation > 0,
		Entitlement: entitlement,
		Limit:       &allocation,
		UsagePeriod: &usagePeriod,
	}
	if softOk {
		if soft >= 0 && soft < allocation {
			feature.SoftLimit = &soft
		} else {
			ignoredClaims = append(ignoredClaims, ClaimAgentRuntimeHoursLimitSoft)
		}
	}
	if hardOk {
		if allocation > 0 && hard >= allocation {
			feature.HardLimit = &hard
		} else {
			ignoredClaims = append(ignoredClaims, ClaimAgentRuntimeHoursLimitHard)
		}
	}
	return feature, true, ignoredClaims
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
