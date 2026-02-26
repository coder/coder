import {
	chatModelConfigs,
	chatModels,
	chatProviderConfigs,
	createChatModelConfig as createChatModelConfigMutation,
	createChatProviderConfig as createChatProviderConfigMutation,
	deleteChatModelConfig as deleteChatModelConfigMutation,
	updateChatModelConfig as updateChatModelConfigMutation,
	updateChatProviderConfig as updateChatProviderConfigMutation,
} from "api/queries/chats";
import type * as TypesGen from "api/typesGenerated";
import { Alert, AlertDetail, AlertTitle } from "components/Alert/Alert";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Loader2Icon } from "lucide-react";
import { type FC, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { cn } from "utils/cn";
import { formatProviderLabel } from "../modelOptions";
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
): readonly ProviderState[] =>
	useMemo(() => {
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

		const modelConfigsByProvider = new Map<
			string,
			TypesGen.ChatModelConfig[]
		>();
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
			const modelConfigsForProvider =
				modelConfigsByProvider.get(provider) ?? [];
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
					? hasProviderEntryAPIKey
					: hasManagedAPIKey || hasCatalogAPIKey,
				isEnvPreset,
				baseURL: getProviderBaseURL(providerConfigEntry),
			};
		});
	}, [modelConfigs, catalogData, providerConfigsData]);

// ── Component ──────────────────────────────────────────────────

type ChatModelAdminPanelProps = {
	className?: string;
	section?: ChatModelAdminSection;
};

export const ChatModelAdminPanel: FC<ChatModelAdminPanelProps> = ({
	className,
	section = "providers",
}) => {
	const queryClient = useQueryClient();
	const [requestedProvider, setRequestedProvider] = useState<string | null>(
		null,
	);

	// ── Queries ────────────────────────────────────────────────
	const providerConfigsQuery = useQuery(chatProviderConfigs());
	const modelConfigsQuery = useQuery(chatModelConfigs());
	const modelCatalogQuery = useQuery(chatModels());

	// ── Mutations ──────────────────────────────────────────────
	const createProviderMut = useMutation(
		createChatProviderConfigMutation(queryClient),
	);
	const updateProviderMut = useMutation(
		updateChatProviderConfigMutation(queryClient),
	);
	const createModelMut = useMutation(
		createChatModelConfigMutation(queryClient),
	);
	const updateModelMut = useMutation(
		updateChatModelConfigMutation(queryClient),
	);
	const deleteModelMut = useMutation(
		deleteChatModelConfigMutation(queryClient),
	);

	// ── Sorted model configs ───────────────────────────────────
	const modelConfigs = useMemo(
		() =>
			(modelConfigsQuery.data ?? []).slice().sort((a, b) => {
				const cmp = a.provider.localeCompare(b.provider);
				return cmp !== 0 ? cmp : a.model.localeCompare(b.model);
			}),
		[modelConfigsQuery.data],
	);

	// ── Provider states ────────────────────────────────────────
	const providerStates = useProviderStates(
		modelConfigs,
		providerConfigsQuery.data,
		modelCatalogQuery.data,
	);

	// Derive the effective selected provider from user intent + available
	// providers. This avoids a useEffect + setState cycle that would cause
	// an extra render with a stale value.
	const selectedProvider = useMemo(() => {
		if (
			requestedProvider &&
			providerStates.some((ps) => ps.provider === requestedProvider)
		) {
			return requestedProvider;
		}
		return providerStates[0]?.provider ?? null;
	}, [requestedProvider, providerStates]);

	const selectedProviderState = useMemo(
		() =>
			selectedProvider
				? (providerStates.find((ps) => ps.provider === selectedProvider) ??
					null)
				: null,
		[providerStates, selectedProvider],
	);

	// ── Derived state ──────────────────────────────────────────
	const isLoading =
		providerConfigsQuery.isLoading ||
		modelConfigsQuery.isLoading ||
		modelCatalogQuery.isLoading;
	const providerConfigsUnavailable = providerConfigsQuery.data === null;
	const modelConfigsUnavailable = modelConfigsQuery.data === null;
	const isProviderMutationPending =
		createProviderMut.isPending || updateProviderMut.isPending;
	const providerMutationError =
		createProviderMut.error ?? updateProviderMut.error;
	const modelMutationError =
		createModelMut.error ?? updateModelMut.error ?? deleteModelMut.error;

	return (
		<div className={cn("space-y-3", className)}>
			{/* Header */}
			<div className="flex items-center justify-between gap-4">
				<p className="m-0 text-[13px] leading-relaxed text-content-secondary">
					{section === "providers"
						? "Configure provider credentials and network settings."
						: "Manage models available in Agents across all providers."}
				</p>
				{isLoading && (
					<div className="flex items-center gap-1.5 text-xs text-content-secondary">
						<Loader2Icon className="h-4 w-4 animate-spin" />
						Loading
					</div>
				)}
			</div>

			{/* Alerts */}
			{providerConfigsQuery.isError && (
				<ErrorAlert error={providerConfigsQuery.error} />
			)}
			{modelConfigsQuery.isError && (
				<ErrorAlert error={modelConfigsQuery.error} />
			)}
			{modelCatalogQuery.isError && (
				<ErrorAlert error={modelCatalogQuery.error} />
			)}
			{providerMutationError && <ErrorAlert error={providerMutationError} />}
			{modelMutationError && <ErrorAlert error={modelMutationError} />}

			{providerConfigsUnavailable && (
				<Alert severity="info" className="mb-3">
					<AlertTitle>
						Chat provider admin API is unavailable on this deployment.
					</AlertTitle>
					<AlertDetail>/api/v2/chats/providers is missing.</AlertDetail>
				</Alert>
			)}

			{modelConfigsUnavailable && (
				<Alert severity="info" className="mb-3">
					<AlertTitle>
						Chat model admin API is unavailable on this deployment.
					</AlertTitle>
					<AlertDetail>/api/v2/chats/model-configs is missing.</AlertDetail>
				</Alert>
			)}

			{/* Content */}
			{section === "providers" ? (
				<ProvidersSection
					providerStates={providerStates}
					providerConfigsUnavailable={providerConfigsUnavailable}
					isProviderMutationPending={isProviderMutationPending}
					onCreateProvider={(req) => createProviderMut.mutateAsync(req)}
					onUpdateProvider={(providerConfigId, req) =>
						updateProviderMut.mutateAsync({
							providerConfigId,
							req,
						})
					}
					onSelectedProviderChange={setRequestedProvider}
				/>
			) : (
				<ModelsSection
					providerStates={providerStates}
					selectedProvider={selectedProvider}
					selectedProviderState={selectedProviderState}
					onSelectedProviderChange={setRequestedProvider}
					modelConfigs={modelConfigs}
					modelConfigsUnavailable={modelConfigsUnavailable}
					isCreating={createModelMut.isPending}
					isUpdating={updateModelMut.isPending}
					isDeleting={deleteModelMut.isPending}
					onCreateModel={(req) => createModelMut.mutateAsync(req)}
					onUpdateModel={(modelConfigId, req) =>
						updateModelMut.mutateAsync({
							modelConfigId,
							req,
						})
					}
					onDeleteModel={(id) => deleteModelMut.mutateAsync(id)}
				/>
			)}
		</div>
	);
};
