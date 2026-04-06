import type { ChatProviderConfig } from "#/api/typesGenerated";
import { isDatabaseProviderConfig } from "./helpers";

type ProviderAvailability = {
	hasManagedAPIKey: boolean;
	hasCatalogAPIKey: boolean;
	hasProviderEntryAPIKey: boolean;
	hasDatabaseProviderConfig: boolean;
};

export const hasEnabledDatabaseProviderAPIKey = (
	providerConfigs: readonly ChatProviderConfig[],
): boolean => {
	return providerConfigs.some(
		(config) =>
			isDatabaseProviderConfig(config) &&
			config.enabled &&
			(config.has_api_key || config.has_effective_api_key),
	);
};

/**
 * Reports whether a provider family has any usable API key source.
 * Database-backed configs can still inherit deployment-managed keys
 * surfaced by the provider catalog.
 */
export const hasEffectiveProviderAPIKey = ({
	hasManagedAPIKey,
	hasCatalogAPIKey,
	hasProviderEntryAPIKey,
	hasDatabaseProviderConfig,
}: ProviderAvailability): boolean => {
	if (hasManagedAPIKey || hasCatalogAPIKey) {
		return true;
	}

	return !hasDatabaseProviderConfig && hasProviderEntryAPIKey;
};
