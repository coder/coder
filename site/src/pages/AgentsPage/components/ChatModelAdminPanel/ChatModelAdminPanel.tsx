import type { FC } from "react";

import type * as TypesGen from "#/api/typesGenerated";
import { Alert, AlertDescription, AlertTitle } from "#/components/Alert/Alert";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Spinner } from "#/components/Spinner/Spinner";
import { cn } from "#/utils/cn";
import { formatProviderLabel } from "../../utils/modelOptions";
import {
	getDefaultProviderBaseURL,
	normalizeProvider,
	readOptionalString,
} from "./helpers";
import { ModelsSection } from "./ModelsSection";
import { ProvidersSection } from "./ProvidersSection";

export type CreateProviderResult = { id: string };

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

export type ChatModelAdminSection = "providers" | "models";

type CatalogProvider = TypesGen.ChatModelsResponse["providers"][number];

const nilUUID = "00000000-0000-0000-0000-000000000000";
const envPresetProviders = new Set(["openai", "anthropic"]);

const hasProviderAPIKey = (
	providerConfig: TypesGen.ChatProviderConfig | undefined,
): boolean => {
	if (!providerConfig) return false;
	return providerConfig.has_api_key;
};

const getProviderConfigSource = (
	providerConfig: TypesGen.ChatProviderConfig | undefined,
): TypesGen.ChatProviderConfigSource | undefined => {
	return providerConfig?.source;
};

const isDatabaseProviderConfig = (
	providerConfig: TypesGen.ChatProviderConfig | undefined,
	source: TypesGen.ChatProviderConfigSource | undefined,
): providerConfig is TypesGen.ChatProviderConfig => {
	if (!providerConfig) return false;
	if (providerConfig.id === nilUUID) return false;
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
	if (providerID && providerID !== nilUUID) {
		return providerID;
	}
	return normalizeProvider(providerConfig.provider);
};

type ProviderEntry = {
	key: string;
	provider: string;
};

const useProviderStates = (
	modelConfigs: readonly TypesGen.ChatModelConfig[],
	providerConfigsData: TypesGen.ChatProviderConfig[] | null | undefined,
	catalogData: TypesGen.ChatModelsResponse | null | undefined,
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

	const catalogProviders = getCatalogProviders(catalogData);
	const catalogProvidersByProvider = new Map<string, CatalogProvider>();
	for (const cp of catalogProviders) {
		const provider = normalizeProvider(cp.provider);
		if (!provider) continue;
		catalogProvidersByProvider.set(provider, cp);
	}

	const providerConfigKeysByProvider = new Map<string, string[]>();
	const providerTypesWithConfigs = new Set<string>();
	for (const pc of providerConfigsData ?? []) {
		const provider = normalizeProvider(pc.provider);
		if (!provider) continue;
		const key = providerConfigStateKey(pc);
		providerTypesWithConfigs.add(provider);
		providerConfigKeysByProvider.set(provider, [
			...(providerConfigKeysByProvider.get(provider) ?? []),
			key,
		]);
		includeEntry(key, provider);
	}
	const modelStateKey = (modelConfig: TypesGen.ChatModelConfig): string => {
		const aiProviderID = readOptionalString(modelConfig.ai_provider_id);
		if (aiProviderID) {
			return aiProviderID;
		}
		const provider = normalizeProvider(modelConfig.provider);
		const providerConfigKeys = providerConfigKeysByProvider.get(provider) ?? [];
		if (providerConfigKeys.length === 1) {
			return providerConfigKeys[0];
		}
		return providerConfigKeys.length === 0 ? provider : "";
	};

	for (const cp of catalogProviders) {
		const provider = normalizeProvider(cp.provider);
		if (!provider || providerTypesWithConfigs.has(provider)) continue;
		includeEntry(provider, provider);
	}
	for (const mc of modelConfigs) {
		includeEntry(modelStateKey(mc), mc.provider);
	}

	const providerConfigsByKey = new Map<string, TypesGen.ChatProviderConfig>();
	for (const pc of providerConfigsData ?? []) {
		const key = providerConfigStateKey(pc);
		if (!key) continue;
		providerConfigsByKey.set(key, pc);
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

	return orderedEntries.map(({ key, provider }) => {
		const providerConfigEntry = providerConfigsByKey.get(key);
		const providerConfigSource = getProviderConfigSource(providerConfigEntry);
		const providerConfig = isDatabaseProviderConfig(
			providerConfigEntry,
			providerConfigSource,
		)
			? providerConfigEntry
			: undefined;
		const catalogProvider = catalogProvidersByProvider.get(provider);
		const hasManagedAPIKey = hasProviderAPIKey(providerConfig);
		const hasProviderEntryAPIKey = hasProviderAPIKey(providerConfigEntry);
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
};

interface ChatModelAdminPanelProps {
	className?: string;
	section?: ChatModelAdminSection;
	sectionLabel?: string;
	sectionDescription?: string;
	// Data from queries.
	providerConfigsData: TypesGen.ChatProviderConfig[] | undefined;
	modelConfigsData: TypesGen.ChatModelConfig[] | undefined;
	modelCatalogData: TypesGen.ChatModelsResponse | undefined;
	isLoading: boolean;
	// Query error states.
	providerConfigsError: Error | null;
	modelConfigsError: Error | null;
	modelCatalogError: Error | null;
	// Provider mutation handlers.
	onCreateProvider: (
		req: TypesGen.CreateChatProviderConfigRequest,
	) => Promise<CreateProviderResult>;
	onUpdateProvider: (
		providerConfigId: string,
		req: TypesGen.UpdateChatProviderConfigRequest,
	) => Promise<unknown>;
	onDeleteProvider: (providerConfigId: string) => Promise<void>;
	isProviderMutationPending: boolean;
	providerMutationError: Error | null;
	// Model mutation handlers.
	onCreateModel: (
		req: TypesGen.CreateChatModelConfigRequest,
	) => Promise<unknown>;
	onUpdateModel: (
		modelConfigId: string,
		req: TypesGen.UpdateChatModelConfigRequest,
	) => Promise<unknown>;
	onDeleteModel: (modelConfigId: string) => Promise<void>;
	isCreatingModel: boolean;
	isUpdatingModel: boolean;
	isDeletingModel: boolean;
	modelMutationError: Error | null;
}

export const ChatModelAdminPanel: FC<ChatModelAdminPanelProps> = ({
	className,
	section = "providers",
	sectionLabel,
	sectionDescription,
	providerConfigsData,
	modelConfigsData,
	modelCatalogData,
	isLoading,
	providerConfigsError,
	modelConfigsError,
	modelCatalogError,
	onCreateProvider,
	onUpdateProvider,
	onDeleteProvider,
	isProviderMutationPending,
	providerMutationError,
	onCreateModel,
	onUpdateModel,
	onDeleteModel,
	isCreatingModel,
	isUpdatingModel,
	isDeletingModel,
	modelMutationError,
}) => {
	const modelConfigs = (modelConfigsData ?? []).slice().sort((a, b) => {
		const cmp = a.provider.localeCompare(b.provider);
		return cmp !== 0 ? cmp : a.model.localeCompare(b.model);
	});
	const providerStates = useProviderStates(
		modelConfigs,
		providerConfigsData,
		modelCatalogData,
	);

	const providerConfigsUnavailable = providerConfigsData === null;
	const modelConfigsUnavailable = modelConfigsData === null;

	return (
		<div className={cn("flex min-h-full flex-col", className)}>
			{isLoading && (
				<div className="flex items-center gap-1.5 text-xs text-content-secondary">
					<Spinner className="h-4 w-4" loading />
					Loading
				</div>
			)}

			<div className="flex flex-1 flex-col gap-8">
				{section === "providers" ? (
					<ProvidersSection
						sectionLabel={sectionLabel}
						sectionDescription={sectionDescription}
						providerStates={providerStates}
						providerConfigsUnavailable={providerConfigsUnavailable}
						isProviderMutationPending={isProviderMutationPending}
						onCreateProvider={onCreateProvider}
						onUpdateProvider={onUpdateProvider}
						onDeleteProvider={onDeleteProvider}
					/>
				) : (
					<ModelsSection
						sectionLabel={sectionLabel}
						sectionDescription={sectionDescription}
						providerStates={providerStates}
						modelConfigs={modelConfigs}
						modelConfigsUnavailable={modelConfigsUnavailable}
						isCreating={isCreatingModel}
						isUpdating={isUpdatingModel}
						isDeleting={isDeletingModel}
						onCreateModel={onCreateModel}
						onUpdateModel={onUpdateModel}
						onDeleteModel={onDeleteModel}
					/>
				)}
			</div>
			{providerConfigsError && <ErrorAlert error={providerConfigsError} />}
			{modelConfigsError && <ErrorAlert error={modelConfigsError} />}
			{modelCatalogError && <ErrorAlert error={modelCatalogError} />}
			{providerMutationError && <ErrorAlert error={providerMutationError} />}
			{modelMutationError && <ErrorAlert error={modelMutationError} />}

			{providerConfigsUnavailable && (
				<Alert severity="info">
					<AlertTitle>
						Chat provider admin API is unavailable on this deployment.
					</AlertTitle>
					<AlertDescription>
						/api/v2/chats/providers is missing.
					</AlertDescription>
				</Alert>
			)}

			{modelConfigsUnavailable && (
				<Alert severity="info">
					<AlertTitle>
						Chat model admin API is unavailable on this deployment.
					</AlertTitle>
					<AlertDescription>
						/api/v2/chats/model-configs is missing.
					</AlertDescription>
				</Alert>
			)}
		</div>
	);
};
