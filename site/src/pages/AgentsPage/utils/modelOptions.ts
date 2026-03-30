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

export const getModelOptionsFromConfigs = (
	configs: readonly TypesGen.ChatModelConfig[] | null | undefined,
	catalog: TypesGen.ChatModelsResponse | null | undefined,
): readonly ModelSelectorOption[] => {
	if (!configs || !catalog) {
		return [];
	}

	const availableProviders = getAvailableProviders(catalog);
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
		options.push({
			id: configID,
			provider,
			model,
			displayName,
			...(contextLimit !== undefined ? { contextLimit } : {}),
		});
	}

	return options.sort((a, b) => {
		const providerCompare = a.provider.localeCompare(b.provider);
		if (providerCompare !== 0) {
			return providerCompare;
		}
		return a.displayName.localeCompare(b.displayName);
	});
};

export const formatProviderLabel = (provider: string): string => {
	const normalized = provider.trim().toLowerCase();
	switch (normalized) {
		case "openai":
			return "OpenAI";
		case "anthropic":
			return "Anthropic";
		case "azure":
			return "Azure OpenAI";
		case "bedrock":
			return "AWS Bedrock";
		case "google":
			return "Google";
		case "openai-compatible":
		case "openai_compatible":
			return "OpenAI-compatible";
		case "openrouter":
			return "OpenRouter";
		case "vercel":
			return "Vercel AI Gateway";
		default:
			if (!normalized) {
				return "Unknown";
			}
			return `${normalized[0].toUpperCase()}${normalized.slice(1)}`;
	}
};

export const getModelSelectorPlaceholder = (
	modelOptions: readonly ModelSelectorOption[],
	isModelCatalogLoading: boolean,
	hasConfiguredModels: boolean,
): string => {
	if (modelOptions.length > 0) {
		return "Select model";
	}
	if (isModelCatalogLoading) {
		return "Loading models...";
	}
	if (hasConfiguredModels) {
		return "No Models Available";
	}
	return "No Models Configured";
};
