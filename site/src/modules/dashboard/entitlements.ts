import type {
	Capability,
	DeploymentCapabilities,
	FeatureName,
} from "#/api/typesGenerated";

export const getFeatureVisibility = (
	features: Record<string, Capability>,
): Record<string, boolean> =>
	Object.fromEntries(
		Object.entries(features).map(([feature, capability]) => [
			feature,
			capability.usable,
		]),
	);

export const selectFeatureVisibility = (
	capabilities: DeploymentCapabilities,
): Record<FeatureName, boolean> => getFeatureVisibility(capabilities.features);
