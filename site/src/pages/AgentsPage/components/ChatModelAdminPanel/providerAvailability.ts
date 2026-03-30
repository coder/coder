type ProviderAvailability = {
	hasManagedAPIKey: boolean;
	hasCatalogAPIKey: boolean;
	hasProviderEntryAPIKey: boolean;
	hasDatabaseProviderConfig: boolean;
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
