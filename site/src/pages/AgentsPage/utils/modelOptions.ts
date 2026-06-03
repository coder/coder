import type * as TypesGen from "#/api/typesGenerated";
import type { ModelSelectorOption } from "../components/ChatElements";
import {
	asNumber,
	asString,
} from "../components/ChatElements/runtimeTypeUtils";

type RuntimeModelRef = {
	readonly provider?: unknown;
	readonly model?: unknown;
};

type ModelRefLike =
	| Pick<TypesGen.ChatModel, "provider" | "model">
	| Pick<TypesGen.ChatModelConfig, "provider" | "model">
	| RuntimeModelRef;

type CatalogModelLike =
	| TypesGen.ChatModel
	| (RuntimeModelRef & {
			readonly id?: unknown;
			readonly display_name?: unknown;
	  });

type CatalogProviderLike = Omit<TypesGen.ChatModelProvider, "models"> & {
	readonly models?: readonly CatalogModelLike[];
};

type ModelCatalogLike = {
	readonly providers?: readonly CatalogProviderLike[];
};

type ModelOptionConfigLike =
	| TypesGen.ChatModelConfig
	| (RuntimeModelRef & {
			readonly id?: unknown;
			readonly display_name?: unknown;
			readonly enabled?: unknown;
			readonly context_limit?: unknown;
	  });

export const hasConfiguredProviderConfigs = (
	providerConfigs: readonly TypesGen.ChatProviderConfig[] | null | undefined,
	catalog: TypesGen.ChatModelsResponse | null | undefined,
): boolean => {
	return countConfiguredProviderConfigs(providerConfigs, catalog) > 0;
};

export const countConfiguredProviderConfigs = (
	providerConfigs: readonly TypesGen.ChatProviderConfig[] | null | undefined,
	catalog: TypesGen.ChatModelsResponse | null | undefined,
): number => {
	const availableProviders = getAvailableProviders(catalog);
	return (
		providerConfigs?.filter((providerConfig) => {
			if (
				providerConfig.source === "supported" ||
				providerConfig.enabled !== true
			) {
				return false;
			}
			const provider = asString(providerConfig.provider).trim().toLowerCase();
			return provider !== "" && availableProviders.has(provider);
		}).length ?? 0
	);
};

export const getNormalizedModelRef = (
	value: ModelRefLike,
): { readonly provider: string; readonly model: string } => {
	const modelRef = value ?? {};
	return {
		provider: asString(modelRef.provider).trim().toLowerCase(),
		model: asString(modelRef.model).trim(),
	};
};

const getCatalogProviders = (
	catalog: ModelCatalogLike | null | undefined,
): readonly CatalogProviderLike[] => {
	const providers = catalog?.providers;
	return Array.isArray(providers) ? providers : [];
};

const getProviderModels = (
	provider: CatalogProviderLike,
): readonly CatalogModelLike[] => {
	const models = provider.models;
	return Array.isArray(models) ? models : [];
};

const isProviderConfiguredInCatalog = (
	provider: CatalogProviderLike,
): boolean => {
	if (getProviderModels(provider).length > 0) {
		return true;
	}
	if (provider.available === true) {
		return true;
	}
	const unavailableReason = asString(provider.unavailable_reason).trim();
	return unavailableReason !== "" && unavailableReason !== "missing_api_key";
};

export const hasConfiguredModelsInCatalog = (
	catalog: ModelCatalogLike | null | undefined,
): boolean => {
	return getCatalogProviders(catalog).some(isProviderConfiguredInCatalog);
};

export const hasUserFixableProviders = (
	catalog: TypesGen.ChatModelsResponse | null | undefined,
): boolean => {
	if (!catalog?.providers) {
		return false;
	}
	return catalog.providers.some(
		(provider) => provider.unavailable_reason === "user_api_key_required",
	);
};

const getAvailableProviders = (
	catalog: TypesGen.ChatModelsResponse | null | undefined,
): ReadonlySet<string> => {
	const availableProviders = new Set<string>();
	for (const provider of getCatalogProviders(catalog)) {
		if (provider.available !== true) {
			continue;
		}
		const providerName = asString(provider.provider).trim().toLowerCase();
		if (providerName) {
			availableProviders.add(providerName);
		}
	}
	return availableProviders;
};

/**
 * Resolves a stored model reference (config ID or legacy
 * "provider:model" string) to the ID of a matching model option.
 * Returns the matched option ID, or an empty string if no match is
 * found.
 */
export const resolveModelOptionId = (
	storedRef: string | null | undefined,
	modelOptions: readonly ModelSelectorOption[],
): string => {
	const normalized = asString(storedRef).trim();
	if (!normalized) {
		return "";
	}

	const directMatch = modelOptions.find((option) => option.id === normalized);
	if (directMatch) {
		return directMatch.id;
	}

	const legacyMatch = modelOptions.find(
		(option) => `${option.provider}:${option.model}` === normalized,
	);
	if (legacyMatch) {
		return legacyMatch.id;
	}

	return "";
};

/** Minimal shape needed to resolve provider display names
 * in the model selector. Both ChatProviderConfig (admin) and
 * UserChatProviderConfig (all users) satisfy this. */
type ProviderDisplayNameSource = {
	readonly id?: string;
	readonly provider_id?: string;
	readonly display_name?: string;
};

export const getModelOptionsFromConfigs = (
	configs: readonly TypesGen.ChatModelConfig[] | null | undefined,
	catalog: TypesGen.ChatModelsResponse | null | undefined,
	providerConfigs?: readonly ProviderDisplayNameSource[] | null,
): readonly ModelSelectorOption[] => {
	if (!configs || !catalog) {
		return [];
	}

	const availableProviders = getAvailableProviders(catalog);

	// Build a lookup from provider config ID to its display name
	// so models can carry the human-readable provider label.
	// ChatProviderConfig uses "id", UserChatProviderConfig uses
	// "provider_id"; accept both.
	const providerDisplayNames = new Map<string, string>();
	if (providerConfigs) {
		for (const pc of providerConfigs) {
			const key = asString(pc.id || pc.provider_id).trim();
			const name = asString(pc.display_name).trim();
			if (key && name) {
				providerDisplayNames.set(key, name);
			}
		}
	}

	const options: ModelSelectorOption[] = [];

	for (const config of configs as readonly ModelOptionConfigLike[]) {
		if (config.enabled !== true) {
			continue;
		}

		const configID = asString(config.id).trim();
		const { provider, model } = getNormalizedModelRef(config);
		if (!configID || !provider || !model) {
			continue;
		}
		if (!availableProviders.has(provider)) {
			continue;
		}

		const displayName = asString(config.display_name).trim() || model;
		const contextLimit = asNumber(config.context_limit);
		const aiProviderID = asString(
			(config as { ai_provider_id?: unknown }).ai_provider_id,
		).trim();
		// Read provider display name from the enriched field stamped by
		// the chatModelConfigs query, or fall back to the separate
		// provider configs lookup.
		const providerDisplayName =
			asString(
				(config as { __providerDisplayName?: unknown })
					.__providerDisplayName,
			).trim() ||
			(aiProviderID
				? providerDisplayNames.get(aiProviderID)
				: undefined) ||
			undefined;

		options.push({
			id: configID,
			provider,
			model,
			displayName,
			...(contextLimit !== undefined ? { contextLimit } : {}),
			...(aiProviderID ? { providerConfigId: aiProviderID } : {}),
			...(providerDisplayName ? { providerDisplayName } : {}),
		});
	}

	return options.sort((a, b) => {
		// Sort by provider config instance first to keep models from the
		// same provider instance together, then by provider type, then
		// by display name.
		const groupA = a.providerConfigId || a.provider;
		const groupB = b.providerConfigId || b.provider;
		const groupCompare = groupA.localeCompare(groupB);
		if (groupCompare !== 0) {
			return groupCompare;
		}
		return a.displayName.localeCompare(b.displayName);
	});
};

// getProviderForModelOption returns the provider string for the
// currently-selected model option, or undefined when the selection
// is not (yet) in the options list. Extracted so resize/budget logic
// has one place to resolve provider from the selector state.
export const getProviderForModelOption = (
	modelOptions: readonly ModelSelectorOption[],
	selectedModel: string,
): string | undefined =>
	modelOptions.find((option) => option.id === selectedModel)?.provider;

export { formatProviderLabel } from "#/utils/aiProviders";

export const getModelSelectorPlaceholder = (
	modelOptions: readonly ModelSelectorOption[],
	isModelCatalogLoading: boolean,
	hasConfiguredModels: boolean,
	catalog?: TypesGen.ChatModelsResponse | null,
): string => {
	if (modelOptions.length > 0) {
		return "Select model";
	}
	if (isModelCatalogLoading) {
		return "Loading models...";
	}
	if (hasConfiguredModels) {
		return hasUserFixableProviders(catalog)
			? "Configure API Keys"
			: "No Models Available";
	}
	return "No Models Configured";
};
