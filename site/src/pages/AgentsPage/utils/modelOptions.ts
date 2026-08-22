import type { UseQueryResult } from "react-query";
import type * as TypesGen from "#/api/typesGenerated";
import type { ModelSelectorOption } from "../components/ChatElements";
import {
	asNumber,
	asString,
} from "../components/ChatElements/runtimeTypeUtils";

type CatalogModelLike =
	| TypesGen.ChatModelCatalogEntry
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
	catalog: TypesGen.ChatModelAvailabilityResponse | null | undefined,
): boolean => {
	return countConfiguredProviderConfigs(providerConfigs, catalog) > 0;
};

export const countConfiguredProviderConfigs = (
	providerConfigs: readonly TypesGen.ChatProviderConfig[] | null | undefined,
	catalog: TypesGen.ChatModelAvailabilityResponse | null | undefined,
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
	catalog: TypesGen.ChatModelAvailabilityResponse | null | undefined,
): boolean => {
	if (!catalog?.providers) {
		return false;
	}
	return catalog.providers.some(
		(provider) => provider.unavailable_reason === "user_api_key_required",
	);
};

const getCatalogUnsupportedProviders = (
	catalog: TypesGen.ChatModelAvailabilityResponse | null | undefined,
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
	catalog: TypesGen.ChatModelAvailabilityResponse | null | undefined,
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
	catalog: TypesGen.ChatModelAvailabilityResponse | null | undefined,
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
 * Resolves a stored ChatModel ID to the ID of a matching model
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
// getModelOptionsFromModels needs. The admin and user provider endpoints
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
 * Drops models whose provider row is disabled or missing. Both
 * provider-info sources include every enabled provider, so a missing row
 * means the provider is disabled or deleted.
 */
export const filterModelsWithEnabledProvider = (
	models: readonly TypesGen.ChatModel[],
	providerInfoByID: ReadonlyMap<string, ProviderInfo>,
): readonly TypesGen.ChatModel[] =>
	models.filter((model) => {
		const info = providerInfoByID.get(model.ai_provider_id);
		return info !== undefined && info.enabled !== false;
	});

export const getModelOptionsFromModels = (
	models: readonly TypesGen.ChatModel[] | null | undefined,
	catalog: TypesGen.ChatModelAvailabilityResponse | null | undefined,
	providerInfoByID: ReadonlyMap<string, ProviderInfo>,
): readonly ModelSelectorOption[] => {
	if (!models || !catalog) {
		return [];
	}

	const availableProviders = getAvailableProviders(catalog);
	const options: ModelSelectorOption[] = [];

	// The catalog check below is keyed by provider type, so it cannot
	// exclude a disabled provider when another of the same type is enabled.
	for (const model of filterModelsWithEnabledProvider(
		models,
		providerInfoByID,
	)) {
		if (!model.enabled) {
			continue;
		}

		const modelID = model.id.trim();
		const providerInfo = providerInfoByID.get(model.ai_provider_id);
		const provider = asString(providerInfo?.provider).trim().toLowerCase();
		const modelName = model.model.trim();
		if (!modelID || !providerInfo || !provider || !modelName) {
			continue;
		}
		if (!availableProviders.has(provider)) {
			continue;
		}

		const displayName = model.display_name.trim() || modelName;
		const contextLimit = asNumber(model.context_limit);
		const reasoningEffort = model.model_config?.reasoning_effort;
		const reasoningEffortDefault = asString(reasoningEffort?.default).trim();
		const reasoningEfforts = model.reasoning_efforts ?? [];
		options.push({
			id: modelID,
			provider,
			providerId: model.ai_provider_id,
			providerLabel: providerInfo.displayName,
			providerIcon: providerInfo.icon,
			model: modelName,
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
	readonly modelCatalog: TypesGen.ChatModelAvailabilityResponse | undefined;
	readonly hasConfiguredModels: boolean;
}

// Provider identity comes from a separate query (userChatProviderModels).
// Folding all three loading states into one flag here spares every caller the
// "models loaded but providers still pending" window that would otherwise
// build an empty provider map, drop every option, and flash "No Models".
export const resolveModelSelector = (
	models: SelectorQuery<readonly TypesGen.ChatModel[]>,
	catalog: SelectorQuery<TypesGen.ChatModelAvailabilityResponse>,
	userProviderModels: SelectorQuery<readonly TypesGen.UserChatProviderConfig[]>,
): ModelSelectorState => ({
	options: getModelOptionsFromModels(
		models.data,
		catalog.data,
		providerInfoByIDFromUserConfigs(userProviderModels.data),
	),
	isModelCatalogLoading:
		models.isLoading || catalog.isLoading || userProviderModels.isLoading,
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
	catalog?: TypesGen.ChatModelAvailabilityResponse | null,
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
