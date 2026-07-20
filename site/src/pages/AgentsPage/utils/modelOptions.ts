import type { UseQueryResult } from "react-query";
import type * as TypesGen from "#/api/typesGenerated";
import type { ModelSelectorOption } from "../components/ChatElements";
import {
	asNumber,
	asString,
} from "../components/ChatElements/runtimeTypeUtils";

type CatalogModelLike =
	| TypesGen.ChatModel
	| {
			readonly id?: unknown;
			readonly display_name?: unknown;
	  };

type CatalogProviderLike = Omit<TypesGen.ChatModelProvider, "models"> & {
	readonly models?: readonly CatalogModelLike[];
};

type ModelCatalogLike = {
	readonly providers?: readonly CatalogProviderLike[];
};

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

const getCatalogUnsupportedProviders = (
	catalog: TypesGen.ChatModelsResponse | null | undefined,
): readonly TypesGen.ChatUnsupportedProvider[] => {
	const unsupported = catalog?.unsupported_providers;
	return Array.isArray(unsupported) ? unsupported : [];
};

/**
 * Display names of configured providers the Agents harness cannot serve,
 * but only when no supported provider is configured. A supported provider
 * missing its API key returns an empty list, keeping normal setup guidance.
 */
export const getUnsupportedProviderNames = (
	catalog: TypesGen.ChatModelsResponse | null | undefined,
): readonly string[] => {
	const unsupported = getCatalogUnsupportedProviders(catalog);
	if (unsupported.length === 0) {
		return [];
	}
	if (getCatalogProviders(catalog).length > 0) {
		return [];
	}
	return unsupported.map(
		(provider) =>
			asString(provider.display_name).trim() ||
			asString(provider.provider).trim(),
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
 * Resolves a stored model config ID to the ID of a matching model
 * option. Returns the matched option ID, or an empty string when the
 * stored ID is blank or no longer matches an available option.
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

	return "";
};

export type ProviderInfo = {
	readonly provider: string;
	readonly displayName: string;
	readonly icon: string;
	// Absent is treated as enabled.
	readonly enabled?: boolean;
};

// providerInfoByIDFromConfigs and providerInfoByIDFromUserConfigs build
// the ai_provider_id -> provider metadata lookup that
// getModelOptionsFromConfigs needs. The admin and user provider endpoints
// expose the provider id under different field names (id vs provider_id), so
// each source has its own helper to bake in the correct field.
export const providerInfoByIDFromConfigs = (
	providerConfigs: readonly TypesGen.ChatProviderConfig[] | null | undefined,
): ReadonlyMap<string, ProviderInfo> =>
	new Map(
		(providerConfigs ?? []).map((providerConfig) => [
			providerConfig.id,
			{
				provider: providerConfig.provider,
				displayName: providerConfig.display_name,
				icon: providerConfig.icon,
				enabled: providerConfig.enabled,
			},
		]),
	);

export const providerInfoByIDFromUserConfigs = (
	providerConfigs:
		| readonly TypesGen.UserChatProviderConfig[]
		| null
		| undefined,
): ReadonlyMap<string, ProviderInfo> =>
	new Map(
		(providerConfigs ?? []).map((providerConfig) => [
			providerConfig.provider_id,
			{
				provider: providerConfig.provider,
				displayName: providerConfig.display_name,
				icon: providerConfig.icon,
				enabled: providerConfig.enabled,
			},
		]),
	);

export const providerTypeByIDFromConfigs = (
	providerConfigs: readonly TypesGen.ChatProviderConfig[] | null | undefined,
): ReadonlyMap<string, string> =>
	new Map(
		Array.from(providerInfoByIDFromConfigs(providerConfigs), ([id, info]) => [
			id,
			info.provider,
		]),
	);

export const providerTypeByIDFromUserConfigs = (
	providerConfigs:
		| readonly TypesGen.UserChatProviderConfig[]
		| null
		| undefined,
): ReadonlyMap<string, string> =>
	new Map(
		Array.from(
			providerInfoByIDFromUserConfigs(providerConfigs),
			([id, info]) => [id, info.provider],
		),
	);

/**
 * Drops model configs whose provider row is disabled or missing. Both
 * provider-info sources include every enabled provider, so a missing row
 * means the provider is disabled or deleted.
 */
export const filterConfigsWithEnabledProvider = (
	configs: readonly TypesGen.ChatModelConfig[],
	providerInfoByID: ReadonlyMap<string, ProviderInfo>,
): readonly TypesGen.ChatModelConfig[] =>
	configs.filter((config) => {
		const info = providerInfoByID.get(config.ai_provider_id);
		return info !== undefined && info.enabled !== false;
	});

export const getModelOptionsFromConfigs = (
	configs: readonly TypesGen.ChatModelConfig[] | null | undefined,
	catalog: TypesGen.ChatModelsResponse | null | undefined,
	providerInfoByID: ReadonlyMap<string, ProviderInfo>,
): readonly ModelSelectorOption[] => {
	if (!configs || !catalog) {
		return [];
	}

	const availableProviders = getAvailableProviders(catalog);
	const options: ModelSelectorOption[] = [];

	// The catalog check below is keyed by provider type, so it cannot
	// exclude a disabled provider when another of the same type is enabled.
	for (const config of filterConfigsWithEnabledProvider(
		configs,
		providerInfoByID,
	)) {
		if (!config.enabled) {
			continue;
		}

		const configID = config.id.trim();
		const providerInfo = providerInfoByID.get(config.ai_provider_id);
		const provider = asString(providerInfo?.provider).trim().toLowerCase();
		const model = config.model.trim();
		if (!configID || !providerInfo || !provider || !model) {
			continue;
		}
		if (!availableProviders.has(provider)) {
			continue;
		}

		const displayName = config.display_name.trim() || model;
		const contextLimit = asNumber(config.context_limit);
		const reasoningEffort = config.model_config?.reasoning_effort;
		const reasoningEffortDefault = asString(reasoningEffort?.default).trim();
		const reasoningEfforts = config.reasoning_efforts ?? [];
		options.push({
			id: configID,
			provider,
			providerId: config.ai_provider_id,
			providerLabel: providerInfo.displayName,
			providerIcon: providerInfo.icon,
			model,
			displayName,
			...(contextLimit !== undefined ? { contextLimit } : {}),
			...(reasoningEffortDefault ? { reasoningEffortDefault } : {}),
			...(reasoningEfforts.length > 0 ? { reasoningEfforts } : {}),
		});
	}

	return options.sort((a, b) => {
		const providerCompare = (a.providerLabel ?? a.provider).localeCompare(
			b.providerLabel ?? b.provider,
		);
		if (providerCompare !== 0) {
			return providerCompare;
		}
		return a.displayName.localeCompare(b.displayName);
	});
};

// Read slice of a react-query result. The field types come from UseQueryResult
// by indexed access, not Pick (which would distribute over v5's status union),
// so they track the library rather than being hand-maintained.
type SelectorQuery<T> = {
	readonly data: UseQueryResult<T>["data"];
	readonly isLoading: UseQueryResult<T>["isLoading"];
};

interface ModelSelectorState {
	readonly options: readonly ModelSelectorOption[];
	readonly isModelCatalogLoading: boolean;
	readonly modelCatalog: TypesGen.ChatModelsResponse | undefined;
	readonly hasConfiguredModels: boolean;
}

// Provider identity comes from a separate query (userChatProviderConfigs).
// Folding all three loading states into one flag here spares every caller the
// "configs loaded but providers still pending" window that would otherwise
// build an empty provider map, drop every option, and flash "No Models".
export const resolveModelSelector = (
	modelConfigs: SelectorQuery<readonly TypesGen.ChatModelConfig[]>,
	catalog: SelectorQuery<TypesGen.ChatModelsResponse>,
	userProviderConfigs: SelectorQuery<
		readonly TypesGen.UserChatProviderConfig[]
	>,
): ModelSelectorState => ({
	options: getModelOptionsFromConfigs(
		modelConfigs.data,
		catalog.data,
		providerInfoByIDFromUserConfigs(userProviderConfigs.data),
	),
	isModelCatalogLoading:
		modelConfigs.isLoading ||
		catalog.isLoading ||
		userProviderConfigs.isLoading,
	modelCatalog: catalog.data,
	hasConfiguredModels: hasConfiguredModelsInCatalog(catalog.data),
});

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
