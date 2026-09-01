import type * as TypesGen from "#/api/typesGenerated";
import { normalizeProvider } from "#/modules/aiModels/helpers";
import type { ModelSelectorOption } from "../components/ChatElements";
import {
	asNumber,
	asString,
} from "../components/ChatElements/runtimeTypeUtils";

export const hasConfiguredProviderConfigs = (
	providerConfigs: readonly TypesGen.ChatProviderConfig[] | null | undefined,
	catalog: TypesGen.OrganizationChatModelsResponse | null | undefined,
): boolean => countConfiguredProviderConfigs(providerConfigs, catalog) > 0;

export const countConfiguredProviderConfigs = (
	providerConfigs: readonly TypesGen.ChatProviderConfig[] | null | undefined,
	catalog: TypesGen.OrganizationChatModelsResponse | null | undefined,
): number => {
	const availableProviderIDs = new Set(
		(catalog?.providers ?? [])
			.filter((provider) => provider.available)
			.map((provider) => provider.id),
	);
	return (
		providerConfigs?.filter(
			(providerConfig) =>
				providerConfig.source !== "supported" &&
				providerConfig.enabled &&
				availableProviderIDs.has(providerConfig.id),
		).length ?? 0
	);
};

const hasConfiguredModelsInCatalog = (
	catalog: TypesGen.OrganizationChatModelsResponse | null | undefined,
): boolean => (catalog?.models.length ?? 0) > 0;

export const hasUserFixableProviders = (
	catalog: TypesGen.OrganizationChatModelsResponse | null | undefined,
): boolean =>
	catalog?.providers.some(
		(provider) => provider.unavailable_reason === "user_api_key_required",
	) ?? false;

/**
 * Display names of configured providers the Agents harness cannot serve,
 * but only when no supported provider is configured. A supported provider
 * missing its API key returns an empty list, keeping normal setup guidance.
 */
export const getUnsupportedProviderNames = (
	catalog: TypesGen.OrganizationChatModelsResponse | null | undefined,
): readonly string[] => {
	const unsupported = catalog?.unsupported_providers ?? [];
	const unsupportedProviderTypes = new Set(
		unsupported.map((provider) => normalizeProvider(provider.provider)),
	);
	const hasSupportedProvider =
		catalog?.providers.some(
			(provider) =>
				provider.enabled &&
				!unsupportedProviderTypes.has(normalizeProvider(provider.type)),
		) ?? false;
	if (unsupported.length === 0 || hasSupportedProvider) {
		return [];
	}
	return unsupported.map(
		(provider) =>
			asString(provider.display_name).trim() ||
			asString(provider.provider).trim(),
	);
};

/**
 * The nil UUID (all zeros) is the backend sentinel for an unset stored
 * model reference. Exported so all model-selection surfaces share one value.
 */
export const NIL_UUID = "00000000-0000-0000-0000-000000000000";

/**
 * Reports whether a stored model reference is unset. A blank string and the
 * nil UUID both mean "no selection", so callers handle them identically.
 */
export const isUnsetModelRef = (ref: string | null | undefined): boolean => {
	const normalized = asString(ref).trim();
	return normalized === "" || normalized === NIL_UUID;
};

/**
 * Resolves a stored ChatModel ID to the ID of a matching model
 * option. Returns the matched option ID, or an empty string when the
 * stored ID is unset or no longer matches an available option.
 */
export const resolveModelOptionId = (
	storedRef: string | null | undefined,
	modelOptions: readonly ModelSelectorOption[],
): string => {
	if (isUnsetModelRef(storedRef)) {
		return "";
	}

	const normalized = asString(storedRef).trim();
	const directMatch = modelOptions.find((option) => option.id === normalized);
	if (directMatch) {
		return directMatch.id;
	}

	return "";
};

/**
 * Reports whether a stored model ID no longer matches an available option.
 * Blank and nil UUID references mean no selection, so they are not unavailable.
 */
export const isUnavailableHistoricalModelID = (
	storedRef: string | null | undefined,
	modelOptions: readonly ModelSelectorOption[],
): boolean =>
	!isUnsetModelRef(storedRef) &&
	resolveModelOptionId(storedRef, modelOptions) === "";

/**
 * Returns the usable default model ID for one organization.
 * The function returns an empty string when the default is unavailable.
 */
export const getUsableDefaultModelIDForOrganization = (
	configs: readonly TypesGen.ChatModel[] | null | undefined,
	modelOptions: readonly ModelSelectorOption[],
	organizationID: string,
): string => {
	if (!organizationID || !configs) {
		return "";
	}
	const defaultConfig = configs.find(
		(config) =>
			config.organization_id === organizationID && config.is_default === true,
	);
	return resolveModelOptionId(defaultConfig?.id, modelOptions);
};

export type ProviderInfo = {
	readonly provider: string;
	readonly displayName: string;
	readonly icon: string;
	// Absent is treated as enabled.
	readonly enabled?: boolean;
	readonly available?: boolean;
	readonly unavailableReason?: TypesGen.ChatModelProviderUnavailableReason;
};

// Provider descriptors use `id`; personal provider configs use `provider_id`.
export const providerInfoByIDFromDescriptors = (
	providerDescriptors:
		| readonly TypesGen.ChatModelProviderDescriptor[]
		| null
		| undefined,
): ReadonlyMap<string, ProviderInfo> =>
	new Map(
		(providerDescriptors ?? []).map((providerDescriptor) => [
			providerDescriptor.id,
			{
				provider: providerDescriptor.type,
				displayName: providerDescriptor.display_name,
				icon: providerDescriptor.icon,
				enabled: providerDescriptor.enabled,
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
 * Drops models whose provider row is disabled or missing. Provider descriptors
 * include disabled providers, while user provider configs may omit inaccessible
 * providers.
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
	catalog: TypesGen.OrganizationChatModelsResponse | null | undefined,
	providerInfoByID: ReadonlyMap<string, ProviderInfo>,
	organizationID: string,
): readonly ModelSelectorOption[] => {
	if (!models || !catalog || !organizationID) {
		return [];
	}

	const availableProviderIDs = new Set(
		catalog.providers
			.filter((provider) => provider.available)
			.map((provider) => provider.id),
	);
	const options: ModelSelectorOption[] = [];
	for (const model of filterModelsWithEnabledProvider(
		models.filter((model) => model.organization_id === organizationID),
		providerInfoByID,
	)) {
		if (!model.enabled) {
			continue;
		}

		const modelID = model.id.trim();
		const providerInfo = providerInfoByID.get(model.ai_provider_id);
		const provider = asString(providerInfo?.provider).trim().toLowerCase();
		const modelName = model.model.trim();
		if (
			!modelID ||
			!providerInfo ||
			!availableProviderIDs.has(model.ai_provider_id) ||
			!provider ||
			!modelName
		) {
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

type SelectorQuery<T> = {
	readonly data: T | undefined;
	readonly isLoading: boolean;
};

interface ModelSelectorState {
	readonly options: readonly ModelSelectorOption[];
	readonly isModelCatalogLoading: boolean;
	readonly modelCatalog: TypesGen.OrganizationChatModelsResponse | undefined;
	readonly hasConfiguredModels: boolean;
}

// Provider identity comes from a separate query (userProviderModels).
// Folding both loading states into one flag here spares every caller the
// "models loaded but providers still pending" window that would otherwise
// build an empty provider map, drop every option, and flash "No Models".
export const resolveModelSelector = (
	organizationID: string,
	catalog: SelectorQuery<TypesGen.OrganizationChatModelsResponse>,
): ModelSelectorState => ({
	options: getModelOptionsFromModels(
		catalog.data?.models,
		catalog.data,
		providerInfoByIDFromDescriptors(catalog.data?.providers),
		organizationID,
	),
	isModelCatalogLoading: catalog.isLoading,
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
	catalog?: TypesGen.OrganizationChatModelsResponse | null,
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
