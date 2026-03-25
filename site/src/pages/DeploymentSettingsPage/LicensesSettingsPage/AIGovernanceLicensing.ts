import dayjs from "dayjs";
import type { GetLicensesResponse } from "#/api/api";
import type { Feature } from "#/api/typesGenerated";

function isPremiumLicense(license: GetLicensesResponse): boolean {
	return license.claims.feature_set?.toLowerCase() === "premium";
}

/**
 * Matches the license card add-on section: Premium license with the
 * ai_governance add-on in claims.
 */
export function licenseShowsAiGovernanceAddOn(
	license: GetLicensesResponse,
): boolean {
	return (
		isPremiumLicense(license) &&
		Boolean(license.claims.addons?.includes("ai_governance"))
	);
}

export function isLicenseApplicableForAiGovernanceOverage(
	license: GetLicensesResponse,
	aiGovernanceUserFeature: Feature | undefined,
): boolean {
	const isExpired = dayjs
		.unix(license.claims.license_expires)
		.isBefore(dayjs());
	const isNotYetValid =
		license.claims.nbf !== undefined &&
		dayjs.unix(license.claims.nbf).isAfter(dayjs());
	const isAiGovernanceEntitlementInGracePeriod =
		aiGovernanceUserFeature?.entitlement === "grace_period";

	return (
		!isNotYetValid && (!isExpired || isAiGovernanceEntitlementInGracePeriod)
	);
}

export function hasAiGovernanceAddOnLicense(
	licenses: GetLicensesResponse[] | undefined,
	aiGovernanceUserFeature: Feature | undefined,
): boolean {
	return (
		licenses?.some(
			(license) =>
				licenseShowsAiGovernanceAddOn(license) &&
				isLicenseApplicableForAiGovernanceOverage(
					license,
					aiGovernanceUserFeature,
				),
		) ?? false
	);
}

/**
 * Best-effort limit from license JWT claims when merged entitlements are not
 * yet available. Uses the same per-license field as the add-on card.
 */
function aiGovernanceLimitFromLicenses(
	licenses: GetLicensesResponse[],
	aiGovernanceUserFeature: Feature | undefined,
): number | undefined {
	const limits = licenses
		.filter(
			(license) =>
				licenseShowsAiGovernanceAddOn(license) &&
				isLicenseApplicableForAiGovernanceOverage(
					license,
					aiGovernanceUserFeature,
				),
		)
		.map((license) => license.claims.features?.ai_governance_user_limit)
		.filter((limit): limit is number => limit !== undefined);
	return limits.length > 0 ? Math.max(...limits) : undefined;
}

/**
 * Resolves the displayed AI Governance user limit for summary charts.
 *
 * When the feature is not entitled yet, JWT claims can still carry the
 * purchased add-on seat limit while entitlements may report limit 0.
 */
export function effectiveAiGovernanceLimitForUsageCard(
	aiGovernanceUserFeature: Feature | undefined,
	licenses: GetLicensesResponse[] | undefined,
): number | undefined {
	const limitFromClaims = licenses
		? aiGovernanceLimitFromLicenses(licenses, aiGovernanceUserFeature)
		: undefined;
	const limitFromEntitlements = aiGovernanceUserFeature?.limit;

	return aiGovernanceUserFeature?.enabled === true
		? (limitFromEntitlements ?? limitFromClaims)
		: (limitFromClaims ?? limitFromEntitlements);
}
