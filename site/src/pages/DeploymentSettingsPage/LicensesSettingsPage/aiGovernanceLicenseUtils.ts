import type { GetLicensesResponse } from "api/api";
import type { Feature } from "api/typesGenerated";

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
		(license.claims.addons ?? []).includes("ai_governance")
	);
}

export function hasAiGovernanceAddOnLicense(
	licenses: GetLicensesResponse[] | undefined,
): boolean {
	return licenses?.some(licenseShowsAiGovernanceAddOn) ?? false;
}

/**
 * Best-effort limit from license JWT claims when merged entitlements are not
 * yet available. Uses the same per-license field as the add-on card.
 */
function aiGovernanceLimitFromLicenses(
	licenses: GetLicensesResponse[],
): number | undefined {
	let best: number | undefined;
	for (const license of licenses) {
		if (!licenseShowsAiGovernanceAddOn(license)) {
			continue;
		}
		const lim = license.claims.features?.ai_governance_user_limit;
		if (lim === undefined) {
			continue;
		}
		if (best === undefined || lim > best) {
			best = lim;
		}
	}
	return best;
}

/**
 * Resolves the displayed AI governance user limit for summary charts.
 *
 * When the feature is not entitled yet, JWT claims can still carry the
 * purchased add-on seat limit while entitlements may report limit 0.
 */
export function effectiveAiGovernanceLimitForUsageCard(
	aiGovernanceUserFeature: Feature | undefined,
	licenses: GetLicensesResponse[] | undefined,
): number | undefined {
	const limitFromClaims = licenses
		? aiGovernanceLimitFromLicenses(licenses)
		: undefined;
	const limitFromEntitlements = aiGovernanceUserFeature?.limit;

	return aiGovernanceUserFeature?.enabled === true
		? (limitFromEntitlements ?? limitFromClaims)
		: (limitFromClaims ?? limitFromEntitlements);
}
