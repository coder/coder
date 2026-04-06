import { type FC, type ReactNode, useState } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { Alert, AlertDescription, AlertTitle } from "#/components/Alert/Alert";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Spinner } from "#/components/Spinner/Spinner";
import { cn } from "#/utils/cn";
import { formatProviderLabel } from "../../utils/modelOptions";
import {
	isDatabaseProviderConfig,
	normalizeProvider,
	readOptionalString,
} from "./helpers";
import { ModelsSection } from "./ModelsSection";
import { ProvidersSection } from "./ProvidersSection";
import {
	hasEffectiveProviderAPIKey,
	hasEnabledDatabaseProviderAPIKey,
} from "./providerAvailability";

// ── Exported types ─────────────────────────────────────────────

export type ProviderState = {
	provider: string;
	label: string;
	providerConfigs: readonly TypesGen.ChatProviderConfig[];
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

const envPresetProviders = new Set(["openai", "anthropic"]);

const hasProviderAPIKey = (
	providerConfig: TypesGen.ChatProviderConfig | undefined,
): boolean => {
	if (!providerConfig) return false;
	return providerConfig.has_api_key;
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
		TypesGen.ChatProviderConfig[]
	>();
	for (const pc of providerConfigsData ?? []) {
		const normalized = normalizeProvider(pc.provider);
		if (!normalized) continue;
		const existing = providerConfigsByProvider.get(normalized);
		if (existing) {
			existing.push(pc);
		} else {
			providerConfigsByProvider.set(normalized, [pc]);
		}
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
		const providerConfigsForFamily =
			providerConfigsByProvider.get(provider) ?? [];
		const databaseConfigs = providerConfigsForFamily.filter(
			isDatabaseProviderConfig,
		);
		const enabledDatabaseConfigs = databaseConfigs.filter(
			(config) => config.enabled,
		);
		const effectiveProviderConfig =
			enabledDatabaseConfigs[0] ?? databaseConfigs[0];
		const firstProviderEntry = providerConfigsForFamily[0];

		const catalogProvider = catalogProvidersByProvider.get(provider);
		const catalogProviderSource = readOptionalString(
			(catalogProvider as CatalogProvider & { source?: string })?.source,
		);
		const hasManagedAPIKey = hasEnabledDatabaseProviderAPIKey(
			providerConfigsForFamily,
		);
		const hasProviderEntryAPIKey = hasProviderAPIKey(firstProviderEntry);
		const hasCatalogAPIKey = catalogProvider
			? providerHasCatalogAPIKey(catalogProvider)
			: false;
		const label =
			readOptionalString(firstProviderEntry?.display_name) ??
			formatProviderLabel(provider);
		const modelConfigsForProvider = modelConfigsByProvider.get(provider) ?? [];
		const isCatalogEnvPreset =
			enabledDatabaseConfigs.length === 0 &&
			envPresetProviders.has(provider) &&
			(catalogProviderSource === "env" || hasCatalogAPIKey);
		const isEnvPreset =
			firstProviderEntry?.source === "env_preset" || isCatalogEnvPreset;

		const hasEffectiveAPIKey = hasEffectiveProviderAPIKey({
			hasManagedAPIKey,
			hasCatalogAPIKey,
			hasProviderEntryAPIKey,
			hasDatabaseProviderConfig: Boolean(effectiveProviderConfig),
		});

		return {
			provider,
			label,
			providerConfigs: providerConfigsForFamily,
			modelConfigs: modelConfigsForProvider,
			catalogModelCount: getProviderModels(catalogProvider).length,
			hasManagedAPIKey,
			hasCatalogAPIKey,
			hasEffectiveAPIKey,
			isEnvPreset,
			baseURL: getProviderBaseURL(effectiveProviderConfig),
		};
	});
};

async function missingOnCreateProviderHandler(
	_req: TypesGen.CreateChatProviderConfigRequest,
): Promise<TypesGen.ChatProviderConfig> {
	throw new Error("onCreateProvider handler is required but was not provided.");
}

// ── Component ──────────────────────────────────────────────────

interface ChatModelAdminPanelProps {
	className?: string;
	section?: ChatModelAdminSection;
	sectionLabel?: string;
	sectionDescription?: string;
	sectionBadge?: ReactNode;
	// Data props (optional for backward compat with stories).
	providerConfigsData?: TypesGen.ChatProviderConfig[] | null;
	modelConfigsData?: TypesGen.ChatModelConfig[] | null;
	catalogData?: TypesGen.ChatModelsResponse | null;
	isLoading?: boolean;
	providerConfigsUnavailable?: boolean;
	modelConfigsUnavailable?: boolean;
	providerConfigsError?: unknown;
	modelConfigsError?: unknown;
	catalogError?: unknown;
	// Provider mutation props.
	isProviderMutationPending?: boolean;
	providerMutationError?: unknown;
	onCreateProvider?: (
		req: TypesGen.CreateChatProviderConfigRequest,
	) => Promise<TypesGen.ChatProviderConfig>;
	onUpdateProvider?: (
		providerConfigId: string,
		req: TypesGen.UpdateChatProviderConfigRequest,
	) => Promise<unknown>;
	onDeleteProvider?: (providerConfigId: string) => Promise<void>;
	// Model mutation props.
	isCreatingModel?: boolean;
	isUpdatingModel?: boolean;
	isDeletingModel?: boolean;
	modelMutationError?: unknown;
	onCreateModel?: (
		req: TypesGen.CreateChatModelConfigRequest,
	) => Promise<unknown>;
	onUpdateModel?: (
		modelConfigId: string,
		req: TypesGen.UpdateChatModelConfigRequest,
	) => Promise<unknown>;
	onDeleteModel?: (modelConfigId: string) => Promise<void>;
}

export const ChatModelAdminPanel: FC<ChatModelAdminPanelProps> = ({
	className,
	section = "providers",
	sectionLabel,
	sectionDescription,
	sectionBadge,
	providerConfigsData = undefined,
	modelConfigsData = undefined,
	catalogData = undefined,
	isLoading = false,
	providerConfigsUnavailable = false,
	modelConfigsUnavailable = false,
	providerConfigsError = undefined,
	modelConfigsError = undefined,
	catalogError = undefined,
	isProviderMutationPending = false,
	providerMutationError = undefined,
	onCreateProvider = missingOnCreateProviderHandler,
	onUpdateProvider = async () => {},
	onDeleteProvider = async () => {},
	isCreatingModel = false,
	isUpdatingModel = false,
	isDeletingModel = false,
	modelMutationError = undefined,
	onCreateModel = async () => {},
	onUpdateModel = async () => {},
	onDeleteModel = async () => {},
}) => {
	const [requestedProvider, setRequestedProvider] = useState<string | null>(
		null,
	);
	const [selectedModelOptionKey, setSelectedModelOptionKey] = useState<
		string | null
	>(null);

	// ── Sorted model configs ───────────────────────────────────
	const modelConfigs = (modelConfigsData ?? []).slice().sort((a, b) => {
		const cmp = a.provider.localeCompare(b.provider);
		return cmp !== 0 ? cmp : a.model.localeCompare(b.model);
	});

	// ── Provider states ────────────────────────────────────────
	const providerStates = useProviderStates(
		modelConfigs,
		providerConfigsData,
		catalogData,
	);

	// Derive the effective selected provider from user intent + available
	// providers. This avoids a useEffect + setState cycle that would cause
	// an extra render with a stale value.
	const selectedProvider =
		requestedProvider &&
		providerStates.some((ps) => ps.provider === requestedProvider)
			? requestedProvider
			: (providerStates[0]?.provider ?? null);

	const selectedProviderState = selectedProvider
		? (providerStates.find((ps) => ps.provider === selectedProvider) ?? null)
		: null;

	return (
		<div className={cn("flex min-h-full flex-col space-y-3", className)}>
			{isLoading && (
				<div className="flex items-center gap-1.5 text-xs text-content-secondary">
					<Spinner className="h-4 w-4" loading />
					Loading
				</div>
			)}

			{/* Content */}
			<div className="flex flex-1 flex-col">
				{section === "providers" ? (
					<ProvidersSection
						sectionLabel={sectionLabel}
						sectionDescription={sectionDescription}
						sectionBadge={sectionBadge}
						providerStates={providerStates}
						providerConfigsUnavailable={providerConfigsUnavailable}
						isLoading={isLoading}
						isProviderMutationPending={isProviderMutationPending}
						onCreateProvider={(req) => onCreateProvider(req)}
						onUpdateProvider={(providerConfigId, req) =>
							onUpdateProvider(providerConfigId, req)
						}
						onDeleteProvider={(id) => onDeleteProvider(id)}
						onSelectedProviderChange={setRequestedProvider}
					/>
				) : (
					<ModelsSection
						sectionLabel={sectionLabel}
						sectionDescription={sectionDescription}
						sectionBadge={sectionBadge}
						providerStates={providerStates}
						selectedProvider={selectedProvider}
						selectedProviderState={selectedProviderState}
						onSelectedProviderChange={setRequestedProvider}
						selectedModelOptionKey={selectedModelOptionKey}
						onSelectedModelOptionChange={setSelectedModelOptionKey}
						modelConfigs={modelConfigs}
						modelConfigsUnavailable={modelConfigsUnavailable}
						isLoading={isLoading}
						isCreating={isCreatingModel}
						isUpdating={isUpdatingModel}
						isDeleting={isDeletingModel}
						onCreateModel={(req) => onCreateModel(req)}
						onUpdateModel={(modelConfigId, req) =>
							onUpdateModel(modelConfigId, req)
						}
						onDeleteModel={(id) => onDeleteModel(id)}
					/>
				)}
			</div>

			{/* Errors — rendered at the bottom */}
			{providerConfigsError ? (
				<ErrorAlert error={providerConfigsError} />
			) : null}
			{modelConfigsError ? <ErrorAlert error={modelConfigsError} /> : null}
			{catalogError ? <ErrorAlert error={catalogError} /> : null}
			{providerMutationError ? (
				<ErrorAlert error={providerMutationError} />
			) : null}
			{modelMutationError ? <ErrorAlert error={modelMutationError} /> : null}

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
