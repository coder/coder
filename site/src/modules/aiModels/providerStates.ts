import type * as TypesGen from "#/api/typesGenerated";
import {
	getDefaultProviderBaseURL,
	normalizeProvider,
} from "#/modules/aiModels/helpers";
import { formatProviderLabel } from "#/utils/aiProviders";

export type ProviderState = {
	key: string;
	provider: string;
	label: string;
	providerConfig: TypesGen.ChatProviderConfig | undefined;
	modelConfigs: readonly TypesGen.ChatModelConfig[];
	catalogModelCount: number;
	hasManagedAPIKey: boolean;
	hasCatalogAPIKey: boolean;
	hasEffectiveAPIKey: boolean;
	allowUserAPIKey: boolean;
	isEnvPreset: boolean;
	baseURL: string;
};

type CatalogProvider = TypesGen.ChatModelsResponse["providers"][number];

const envPresetProviders = new Set(["openai", "anthropic"]);

const readOptionalString = (value: unknown): string | undefined => {
	if (typeof value !== "string") return undefined;
	const trimmed = value.trim();
	return trimmed || undefined;
};

const isDatabaseProviderConfig = (
	providerConfig: TypesGen.ChatProviderConfig | undefined,
	source: TypesGen.ChatProviderConfigSource | undefined,
): providerConfig is TypesGen.ChatProviderConfig => {
	if (!providerConfig) return false;
	if (providerConfig.id === "00000000-0000-0000-0000-000000000000") {
		return false;
	}
	return source === undefined || source === "database";
};

const getCatalogProviders = (
	catalog: TypesGen.ChatModelsResponse | null | undefined,
): readonly CatalogProvider[] => {
	const providers = catalog?.providers;
	return Array.isArray(providers) ? providers : [];
};

const providerHasCatalogAPIKey = (provider: CatalogProvider): boolean =>
	provider.available ||
	(Boolean(provider.unavailable_reason) &&
		provider.unavailable_reason !== "missing_api_key");

const getProviderModels = (
	provider: CatalogProvider | undefined,
): readonly CatalogProvider["models"][number][] => {
	const models = provider?.models;
	return Array.isArray(models) ? models : [];
};

const getProviderBaseURL = (
	providerConfig: TypesGen.ChatProviderConfig | undefined,
): string => {
	return (
		readOptionalString(providerConfig?.base_url) ??
		getDefaultProviderBaseURL(providerConfig?.provider ?? "")
	);
};

const providerConfigStateKey = (
	providerConfig: TypesGen.ChatProviderConfig,
): string => {
	const providerID = readOptionalString(providerConfig.id);
	if (providerID && providerID !== "00000000-0000-0000-0000-000000000000") {
		return providerID;
	}
	return normalizeProvider(providerConfig.provider);
};

type ProviderEntry = {
	key: string;
	provider: string;
};

// Returns provider states ordered alphabetically by display label.
export const deriveProviderStates = (
	modelConfigs: readonly TypesGen.ChatModelConfig[],
	providerConfigs: TypesGen.ChatProviderConfig[] | null | undefined,
	catalog: TypesGen.ChatModelsResponse | null | undefined,
): readonly ProviderState[] => {
	const orderedEntries: ProviderEntry[] = [];
	const seenEntries = new Set<string>();
	const includeEntry = (keyValue: string, providerValue: string) => {
		const key = readOptionalString(keyValue);
		const provider = normalizeProvider(providerValue);
		if (!key || !provider || seenEntries.has(key)) return;
		seenEntries.add(key);
		orderedEntries.push({ key, provider });
	};

	const catalogProviders = getCatalogProviders(catalog);
	const catalogProvidersByProvider = new Map<string, CatalogProvider>();
	for (const cp of catalogProviders) {
		const provider = normalizeProvider(cp.provider);
		if (!provider) continue;
		catalogProvidersByProvider.set(provider, cp);
	}

	const providerTypesWithConfigs = new Set<string>();
	const providerConfigsByKey = new Map<string, TypesGen.ChatProviderConfig>();
	for (const pc of providerConfigs ?? []) {
		const provider = normalizeProvider(pc.provider);
		if (!provider) continue;
		const key = providerConfigStateKey(pc);
		providerTypesWithConfigs.add(provider);
		if (key) {
			providerConfigsByKey.set(key, pc);
		}
		includeEntry(key, provider);
	}
	const modelStateKey = (modelConfig: TypesGen.ChatModelConfig): string =>
		readOptionalString(modelConfig.ai_provider_id) ?? "";

	for (const cp of catalogProviders) {
		const provider = normalizeProvider(cp.provider);
		if (!provider || providerTypesWithConfigs.has(provider)) continue;
		includeEntry(provider, provider);
	}
	for (const mc of modelConfigs) {
		const key = modelStateKey(mc);
		includeEntry(key, providerConfigsByKey.get(key)?.provider ?? "");
	}

	const modelConfigsByKey = new Map<string, TypesGen.ChatModelConfig[]>();
	for (const mc of modelConfigs) {
		const key = modelStateKey(mc);
		if (!key) continue;
		const existing = modelConfigsByKey.get(key);
		if (existing) {
			existing.push(mc);
		} else {
			modelConfigsByKey.set(key, [mc]);
		}
	}

	const states = orderedEntries.map(({ key, provider }) => {
		const providerConfigEntry = providerConfigsByKey.get(key);
		const providerConfigSource = providerConfigEntry?.source;
		const providerConfig = isDatabaseProviderConfig(
			providerConfigEntry,
			providerConfigSource,
		)
			? providerConfigEntry
			: undefined;
		const catalogProvider = catalogProvidersByProvider.get(provider);
		const hasManagedAPIKey = providerConfig?.has_api_key ?? false;
		const hasProviderEntryAPIKey = providerConfigEntry?.has_api_key ?? false;
		const hasCatalogAPIKey = catalogProvider
			? providerHasCatalogAPIKey(catalogProvider)
			: false;
		const label =
			readOptionalString(providerConfigEntry?.display_name) ??
			formatProviderLabel(provider);
		const hasBedrockAmbientCredentials =
			provider === "bedrock" &&
			providerConfig?.central_api_key_enabled === true;
		const modelConfigsForProvider = modelConfigsByKey.get(key) ?? [];
		const isCatalogEnvPreset =
			!providerConfig && envPresetProviders.has(provider) && hasCatalogAPIKey;
		const isEnvPreset =
			providerConfigSource === "env_preset" || isCatalogEnvPreset;

		return {
			key,
			provider,
			label,
			providerConfig,
			modelConfigs: modelConfigsForProvider,
			catalogModelCount: getProviderModels(catalogProvider).length,
			hasManagedAPIKey,
			hasCatalogAPIKey,
			hasEffectiveAPIKey: providerConfigEntry
				? hasProviderEntryAPIKey || hasBedrockAmbientCredentials
				: hasManagedAPIKey || hasCatalogAPIKey,
			allowUserAPIKey: providerConfigEntry?.allow_user_api_key ?? true,
			isEnvPreset,
			baseURL: getProviderBaseURL(providerConfigEntry),
		};
	});

	return states.toSorted((a, b) => a.label.localeCompare(b.label));
};

export const canManageProviderModels = (
	providerState: ProviderState | undefined,
): boolean => {
	return Boolean(
		providerState?.providerConfig &&
			providerState.providerConfig.enabled !== false &&
			(providerState.hasEffectiveAPIKey ||
				providerState.providerConfig.allow_user_api_key),
	);
};
