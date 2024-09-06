import type { Entitlements, Feature, FeatureName } from "api/typesGenerated";

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
