import type { Entitlements, Feature, FeatureName } from "#/api/typesGenerated";

/**
 * @param hasLicense true if Enterprise edition
 * @param features record from feature name to feature object
 * @returns record from feature name whether to show the feature
 */
export const getFeatureVisibility = (
	hasLicense: boolean,
	features: Record<string, Feature>,
): Record<string, boolean> => {
	if (!hasLicense) {
		return {};
	}

	const permissionPairs = Object.entries(features).map(
		([feature, { entitlement, limit, actual, enabled }]) => {
			const entitled = ["entitled", "grace_period"].includes(entitlement);
			const limitCompliant = limit && actual ? limit >= actual : true;
			return [feature, entitled && limitCompliant && enabled];
		},
	);
	return Object.fromEntries(permissionPairs);
};

export const selectFeatureVisibility = (
	entitlements: Entitlements,
): Record<FeatureName, boolean> => {
	return getFeatureVisibility(entitlements.has_license, entitlements.features);
};

/**
 * Keep the AI seats column visible while in grace period so admins can
 * identify who is consuming seats while remediating overages.
 */
export const shouldShowAISeatColumn = (entitlements: Entitlements): boolean => {
	const aiGovernanceUserLimit = entitlements.features.ai_governance_user_limit;
	return (
		entitlements.has_license &&
		aiGovernanceUserLimit.enabled &&
		(aiGovernanceUserLimit.entitlement === "entitled" ||
			aiGovernanceUserLimit.entitlement === "grace_period")
	);
};
