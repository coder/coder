import type { FC } from "react";

import type * as TypesGen from "#/api/typesGenerated";
import { Alert, AlertDescription, AlertTitle } from "#/components/Alert/Alert";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Spinner } from "#/components/Spinner/Spinner";
import { cn } from "#/utils/cn";
import { formatProviderLabel } from "../../utils/modelOptions";
import { normalizeProvider, readOptionalString } from "./helpers";
import { ModelsSection } from "./ModelsSection";
import { ProvidersSection } from "./ProvidersSection";

// ── Exported types ─────────────────────────────────────────────

export type ProviderState = {
	provider: string;
	label: string;
	providerConfig: TypesGen.ChatProviderConfig | undefined;
	modelConfigs: readonly TypesGen.ChatModelConfig[];
	catalogModelCount: number;
	hasManagedAPIKey: boolean;
	hasCatalogAPIKey: boolean;
	hasEffectiveAPIKey: boolean;
	isEnvPreset: boolean;
	baseURL: string;
};

export type ChatModelAdminSection = "providers" | "models";

// ── Internal helpers ───────────────────────────────────────────

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
	return readOptionalString(providerConfig?.base_url) ?? "";
};

// ── Hook: compute provider states from query data ──────────────

const useProviderStates = (
	modelConfigs: readonly TypesGen.ChatModelConfig[],
	providerConfigsData: TypesGen.ChatProviderConfig[] | null | undefined,
	catalogData: TypesGen.ChatModelsResponse | null | undefined,
): readonly ProviderState[] => {
	const orderedProviders: string[] = [];
	const seenProviders = new Set<string>();
	const includeProvider = (providerValue: string) => {
		const normalized = normalizeProvider(providerValue);
		if (!normalized || seenProviders.has(normalized)) return;
		seenProviders.add(normalized);
		orderedProviders.push(normalized);
	};

	const catalogProviders = getCatalogProviders(catalogData);
	const catalogProvidersByProvider = new Map<string, CatalogProvider>();
	for (const cp of catalogProviders) {
		const normalized = normalizeProvider(cp.provider);
		if (!normalized) continue;
		includeProvider(normalized);
		catalogProvidersByProvider.set(normalized, cp);
	}

	for (const pc of providerConfigsData ?? []) {
		includeProvider(pc.provider);
	}
	for (const mc of modelConfigs) {
		includeProvider(mc.provider);
	}

	const providerConfigsByProvider = new Map<
		string,
		TypesGen.ChatProviderConfig
	>();
	for (const pc of providerConfigsData ?? []) {
		const normalized = normalizeProvider(pc.provider);
		if (!normalized) continue;
		providerConfigsByProvider.set(normalized, pc);
	}

	const modelConfigsByProvider = new Map<string, TypesGen.ChatModelConfig[]>();
	for (const mc of modelConfigs) {
		const normalized = normalizeProvider(mc.provider);
		if (!normalized) continue;
		const existing = modelConfigsByProvider.get(normalized);
		if (existing) {
			existing.push(mc);
		} else {
			modelConfigsByProvider.set(normalized, [mc]);
		}
	}

	return orderedProviders.map((provider) => {
		const providerConfigEntry = providerConfigsByProvider.get(provider);
		const providerConfigSource = getProviderConfigSource(providerConfigEntry);
		const providerConfig = isDatabaseProviderConfig(
			providerConfigEntry,
			providerConfigSource,
		)
			? providerConfigEntry
			: undefined;
		const catalogProvider = catalogProvidersByProvider.get(provider);
		const catalogProviderSource = readOptionalString(
			(catalogProvider as CatalogProvider & { source?: string })?.source,
		);
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
		const modelConfigsForProvider = modelConfigsByProvider.get(provider) ?? [];
		const isCatalogEnvPreset =
			!providerConfig &&
			envPresetProviders.has(provider) &&
			(catalogProviderSource === "env" || hasCatalogAPIKey);
		const isEnvPreset =
			providerConfigSource === "env_preset" || isCatalogEnvPreset;

		return {
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
			isEnvPreset,
			baseURL: getProviderBaseURL(providerConfigEntry),
		};
	});
};

// ── Component ──────────────────────────────────────────────────

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
	) => Promise<unknown>;
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
	// ── Sorted model configs ───────────────────────────────────
	const modelConfigs = (modelConfigsData ?? []).slice().sort((a, b) => {
		const cmp = a.provider.localeCompare(b.provider);
		return cmp !== 0 ? cmp : a.model.localeCompare(b.model);
	});

	// ── Provider states ────────────────────────────────────────
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

			{/* Content */}
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

			{/* Errors — rendered at the bottom */}
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
