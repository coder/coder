import type {
	ChatModelConfig,
	ChatModelsResponse,
	ChatProviderConfig,
	CreateChatModelConfigRequest,
	CreateChatProviderConfigRequest,
	UpdateChatProviderConfigRequest,
} from "api/api";
import {
	chatModelConfigs,
	chatModels,
	chatProviderConfigs,
	createChatModelConfig as createChatModelConfigMutation,
	createChatProviderConfig as createChatProviderConfigMutation,
	deleteChatModelConfig as deleteChatModelConfigMutation,
	updateChatProviderConfig as updateChatProviderConfigMutation,
} from "api/queries/chats";
import { Alert, AlertDetail, AlertTitle } from "components/Alert/Alert";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Badge } from "components/Badge/Badge";
import { Button } from "components/Button/Button";
import { Input } from "components/Input/Input";
import { Loader2Icon, PlusIcon, Trash2Icon } from "lucide-react";
import {
	type FC,
	type FormEvent,
	useEffect,
	useId,
	useMemo,
	useState,
} from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { cn } from "utils/cn";
import { formatProviderLabel } from "../modelOptions";

type CatalogProvider = ChatModelsResponse["providers"][number];

type ProviderState = {
	provider: string;
	label: string;
	providerConfig: ChatProviderConfig | undefined;
	modelConfigs: readonly ChatModelConfig[];
	catalogModelCount: number;
	hasManagedAPIKey: boolean;
	hasCatalogAPIKey: boolean;
	hasEffectiveAPIKey: boolean;
	isEnvPreset: boolean;
	supportsBaseURL: boolean;
	baseURL: string;
};

type ProviderConfigSource = "database" | "env_preset" | "supported";

const nilUUID = "00000000-0000-0000-0000-000000000000";

const envPresetProviders = new Set(["openai", "anthropic"]);

const normalizeProvider = (provider: string): string =>
	provider.trim().toLowerCase();

const isEnvPresetProvider = (provider: string): boolean =>
	envPresetProviders.has(normalizeProvider(provider));

const hasProviderAPIKey = (providerConfig: ChatProviderConfig | undefined) => {
	if (!providerConfig) {
		return false;
	}
	const providerConfigWithLegacyAPIKeyField =
		providerConfig as ChatProviderConfig & {
			has_api_key?: boolean;
			api_key_set?: boolean;
		};
	return Boolean(
		providerConfigWithLegacyAPIKeyField.api_key_set ??
			providerConfigWithLegacyAPIKeyField.has_api_key,
	);
};

const getProviderConfigSource = (
	providerConfig: ChatProviderConfig | undefined,
): ProviderConfigSource | undefined => {
	const source = readOptionalString(
		(
			providerConfig as ChatProviderConfig & {
				source?: string;
			}
		)?.source,
	);
	switch (source) {
		case "database":
		case "env_preset":
		case "supported":
			return source;
		default:
			return undefined;
	}
};

const isDatabaseProviderConfig = (
	providerConfig: ChatProviderConfig | undefined,
	source: ProviderConfigSource | undefined,
): providerConfig is ChatProviderConfig => {
	if (!providerConfig) {
		return false;
	}
	if (providerConfig.id === nilUUID) {
		return false;
	}
	return source === undefined || source === "database";
};

const getCatalogProviders = (
	catalog: ChatModelsResponse | null | undefined,
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

const readOptionalString = (value: unknown): string | undefined => {
	if (typeof value !== "string") {
		return undefined;
	}
	const trimmedValue = value.trim();
	return trimmedValue || undefined;
};

const readOptionalBoolean = (value: unknown): boolean | undefined => {
	return typeof value === "boolean" ? value : undefined;
};

const providerSupportsBaseURL = (
	provider: string,
	catalogProvider: CatalogProvider | undefined,
	providerConfig: ChatProviderConfig | undefined,
): boolean => {
	const catalogSupportsBaseURL = readOptionalBoolean(
		(
			catalogProvider as CatalogProvider & {
				supports_base_url?: boolean;
			}
		)?.supports_base_url,
	);
	const providerConfigSupportsBaseURL = readOptionalBoolean(
		(
			providerConfig as ChatProviderConfig & {
				supports_base_url?: boolean;
			}
		)?.supports_base_url,
	);
	const providerConfigBaseURL = readOptionalString(
		(
			providerConfig as ChatProviderConfig & {
				base_url?: string;
			}
		)?.base_url,
	);

	return (
		provider.includes("compatible") ||
		Boolean(catalogSupportsBaseURL) ||
		Boolean(providerConfigSupportsBaseURL) ||
		Boolean(providerConfigBaseURL)
	);
};

const getProviderBaseURL = (
	providerConfig: ChatProviderConfig | undefined,
): string => {
	return (
		readOptionalString(
			(
				providerConfig as ChatProviderConfig & {
					base_url?: string;
				}
			)?.base_url,
		) ?? ""
	);
};

type ChatModelAdminPanelProps = {
	className?: string;
};

export const ChatModelAdminPanel: FC<ChatModelAdminPanelProps> = ({
	className,
}) => {
	const queryClient = useQueryClient();
	const [selectedProvider, setSelectedProvider] = useState<string | null>(null);
	const [providerDisplayName, setProviderDisplayName] = useState("");
	const [providerAPIKey, setProviderAPIKey] = useState("");
	const [providerBaseURL, setProviderBaseURL] = useState("");
	const [model, setModel] = useState("");
	const [displayName, setDisplayName] = useState("");
	const providerDisplayNameInputId = useId();
	const providerAPIKeyInputId = useId();
	const providerBaseURLInputId = useId();
	const modelInputId = useId();
	const displayNameInputId = useId();

	const providerConfigsQuery = useQuery(chatProviderConfigs());
	const modelConfigsQuery = useQuery(chatModelConfigs());
	const modelCatalogQuery = useQuery(chatModels());

	const createProviderConfigMutation = useMutation(
		createChatProviderConfigMutation(queryClient),
	);
	const updateProviderConfigMutation = useMutation(
		updateChatProviderConfigMutation(queryClient),
	);
	const createModelConfigMutation = useMutation(
		createChatModelConfigMutation(queryClient),
	);
	const deleteModelConfigMutation = useMutation(
		deleteChatModelConfigMutation(queryClient),
	);

	const modelConfigs = useMemo(
		() =>
			(modelConfigsQuery.data ?? []).slice().sort((a, b) => {
				const providerCompare = a.provider.localeCompare(b.provider);
				if (providerCompare !== 0) {
					return providerCompare;
				}
				return a.model.localeCompare(b.model);
			}),
		[modelConfigsQuery.data],
	);

	const providerStates = useMemo<readonly ProviderState[]>(() => {
		const orderedProviders: string[] = [];
		const seenProviders = new Set<string>();
		const includeProvider = (providerValue: string) => {
			const normalizedProvider = normalizeProvider(providerValue);
			if (!normalizedProvider || seenProviders.has(normalizedProvider)) {
				return;
			}
			seenProviders.add(normalizedProvider);
			orderedProviders.push(normalizedProvider);
		};

		const catalogProviders = getCatalogProviders(modelCatalogQuery.data);
		const catalogProvidersByProvider = new Map<string, CatalogProvider>();
		for (const catalogProvider of catalogProviders) {
			const normalizedProvider = normalizeProvider(catalogProvider.provider);
			if (!normalizedProvider) {
				continue;
			}
			includeProvider(normalizedProvider);
			catalogProvidersByProvider.set(normalizedProvider, catalogProvider);
		}

		for (const providerConfig of providerConfigsQuery.data ?? []) {
			includeProvider(providerConfig.provider);
		}
		for (const modelConfig of modelConfigs) {
			includeProvider(modelConfig.provider);
		}

		const providerConfigsByProvider = new Map<string, ChatProviderConfig>();
		for (const providerConfig of providerConfigsQuery.data ?? []) {
			const normalizedProvider = normalizeProvider(providerConfig.provider);
			if (!normalizedProvider) {
				continue;
			}
			providerConfigsByProvider.set(normalizedProvider, providerConfig);
		}

		const modelConfigsByProvider = new Map<string, ChatModelConfig[]>();
		for (const modelConfig of modelConfigs) {
			const normalizedProvider = normalizeProvider(modelConfig.provider);
			if (!normalizedProvider) {
				continue;
			}
			const existingModels = modelConfigsByProvider.get(normalizedProvider);
			if (existingModels) {
				existingModels.push(modelConfig);
			} else {
				modelConfigsByProvider.set(normalizedProvider, [modelConfig]);
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
				(
					catalogProvider as CatalogProvider & {
						source?: string;
					}
				)?.source,
			);
			const hasManagedAPIKey = hasProviderAPIKey(providerConfig);
			const hasProviderEntryAPIKey = hasProviderAPIKey(providerConfigEntry);
			const hasCatalogAPIKey = catalogProvider
				? providerHasCatalogAPIKey(catalogProvider)
				: false;
			const label =
				readOptionalString(
					(
						providerConfigEntry as ChatProviderConfig & {
							display_name?: string;
						}
					)?.display_name,
				) ??
				readOptionalString(
					(
						catalogProvider as CatalogProvider & {
							display_name?: string;
						}
					)?.display_name,
				) ?? formatProviderLabel(provider);
			const modelConfigsForProvider = modelConfigsByProvider.get(provider) ?? [];
			const isCatalogEnvPreset =
				!providerConfig &&
				isEnvPresetProvider(provider) &&
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
				supportsBaseURL: providerSupportsBaseURL(
					provider,
					catalogProvider,
					providerConfigEntry,
				),
				baseURL: getProviderBaseURL(providerConfigEntry),
			};
		});
	}, [modelConfigs, modelCatalogQuery.data, providerConfigsQuery.data]);

	useEffect(() => {
		setSelectedProvider((current) => {
			if (
				current &&
				providerStates.some((providerState) => providerState.provider === current)
			) {
				return current;
			}
			return providerStates[0]?.provider ?? null;
		});
	}, [providerStates]);

	const selectedProviderState = useMemo(() => {
		if (!selectedProvider) {
			return null;
		}

		return (
			providerStates.find(
				(providerState) => providerState.provider === selectedProvider,
			) ?? null
		);
	}, [providerStates, selectedProvider]);

	useEffect(() => {
		if (!selectedProviderState) {
			setProviderDisplayName("");
			setProviderAPIKey("");
			setProviderBaseURL("");
			return;
		}

		setProviderDisplayName(
			readOptionalString(selectedProviderState.providerConfig?.display_name) ?? "",
		);
		setProviderAPIKey("");
		setProviderBaseURL(selectedProviderState.baseURL);
	}, [selectedProviderState]);

	const selectedProviderModels = selectedProviderState?.modelConfigs ?? [];
	const hasProviderOptions = providerStates.length > 0;
	const providerMutationError =
		createProviderConfigMutation.error ?? updateProviderConfigMutation.error;
	const modelMutationError =
		createModelConfigMutation.error ?? deleteModelConfigMutation.error;
	const isLoading =
		providerConfigsQuery.isLoading ||
		modelConfigsQuery.isLoading ||
		modelCatalogQuery.isLoading;
	const providerConfigsUnavailable = providerConfigsQuery.data === null;
	const modelConfigsUnavailable = modelConfigsQuery.data === null;
	const isProviderMutationPending =
		createProviderConfigMutation.isPending ||
		updateProviderConfigMutation.isPending;
	const requiresProviderAPIKey = !selectedProviderState?.providerConfig;
	const canSaveProviderConfig = Boolean(
		selectedProviderState &&
			!providerConfigsUnavailable &&
			!isProviderMutationPending &&
			(!requiresProviderAPIKey || providerAPIKey.trim()),
	);
	const canManageSelectedProviderModels = Boolean(
		selectedProviderState?.providerConfig &&
			selectedProviderState.hasEffectiveAPIKey,
	);

	const handleSaveProviderConfig = async (event: FormEvent) => {
		event.preventDefault();
		if (
			!selectedProviderState ||
			providerConfigsUnavailable ||
			isProviderMutationPending
		) {
			return;
		}

		const trimmedDisplayName = providerDisplayName.trim();
		const trimmedAPIKey = providerAPIKey.trim();
		const trimmedBaseURL = providerBaseURL.trim();

		if (selectedProviderState.providerConfig) {
			const currentDisplayName =
				readOptionalString(selectedProviderState.providerConfig.display_name) ??
				"";
			const currentBaseURL = selectedProviderState.baseURL.trim();
			const req: UpdateChatProviderConfigRequest & { base_url?: string } = {};

			if (trimmedDisplayName !== currentDisplayName) {
				req.display_name = trimmedDisplayName;
			}
			if (trimmedAPIKey) {
				req.api_key = trimmedAPIKey;
			}
			if (
				selectedProviderState.supportsBaseURL &&
				trimmedBaseURL !== currentBaseURL
			) {
				req.base_url = trimmedBaseURL;
			}
			if (Object.keys(req).length === 0) {
				return;
			}

			await updateProviderConfigMutation.mutateAsync({
				providerConfigId: selectedProviderState.providerConfig.id,
				req,
			});
		} else {
			if (!trimmedAPIKey) {
				return;
			}

			const req: CreateChatProviderConfigRequest & { base_url?: string } = {
				provider: selectedProviderState.provider,
				api_key: trimmedAPIKey,
			};
			if (trimmedDisplayName) {
				req.display_name = trimmedDisplayName;
			}
			if (selectedProviderState.supportsBaseURL && trimmedBaseURL) {
				req.base_url = trimmedBaseURL;
			}

			await createProviderConfigMutation.mutateAsync(req);
		}

		setProviderAPIKey("");
	};

	const handleAddModel = async (event: FormEvent) => {
		event.preventDefault();
		if (
			!selectedProviderState ||
			!selectedProviderState.providerConfig ||
			!selectedProviderState.hasEffectiveAPIKey ||
			createModelConfigMutation.isPending
		) {
			return;
		}

		const trimmedModel = model.trim();
		if (!trimmedModel) {
			return;
		}

		const req: CreateChatModelConfigRequest = {
			provider: selectedProviderState.provider,
			model: trimmedModel,
		};
		const trimmedDisplayName = displayName.trim();
		if (trimmedDisplayName) {
			req.display_name = trimmedDisplayName;
		}

		await createModelConfigMutation.mutateAsync(req);
		setModel("");
		setDisplayName("");
	};

	const handleDeleteModel = async (modelConfigId: string) => {
		if (deleteModelConfigMutation.isPending) {
			return;
		}
		await deleteModelConfigMutation.mutateAsync(modelConfigId);
	};

	return (
		<div
			className={cn(
				"rounded-xl border border-border bg-surface-secondary/40 p-4",
				className,
			)}
		>
			<div className="mb-3 flex items-center justify-between gap-3">
				<div>
					<div className="text-sm font-medium text-content-primary">
						Chat model configuration
					</div>
					<div className="text-xs text-content-secondary">
						Configure providers first, then add or remove models.
					</div>
				</div>
				{isLoading && (
					<div className="flex items-center gap-1 text-2xs text-content-secondary">
						<Loader2Icon className="h-3.5 w-3.5 animate-spin" />
						Loading
					</div>
				)}
			</div>

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

			{!hasProviderOptions ? (
				<div className="rounded-md border border-dashed border-border bg-surface-primary p-3 text-xs text-content-secondary">
					No provider types were returned by the backend.
				</div>
			) : (
				<div className="space-y-3">
					<div className="space-y-2">
						<div className="text-xs font-medium text-content-primary">
							1. Select provider
						</div>
						<div className="grid gap-2 md:grid-cols-2">
							{providerStates.map((providerState) => {
								const isSelected =
									selectedProviderState?.provider === providerState.provider;
								const providerStatus = providerState.providerConfig
									? "Managed config"
									: providerState.isEnvPreset
										? "Environment preset"
										: "Needs API key";
								const providerModelsLabel =
									providerState.modelConfigs.length > 0
										? `${providerState.modelConfigs.length} configured model${providerState.modelConfigs.length === 1 ? "" : "s"}`
										: providerState.catalogModelCount > 0
											? `${providerState.catalogModelCount} catalog model${providerState.catalogModelCount === 1 ? "" : "s"}`
											: "No models configured";

								return (
									<button
										key={providerState.provider}
										type="button"
										className={cn(
											"rounded-lg border px-3 py-2 text-left transition-colors",
											isSelected
												? "border-content-link bg-surface-secondary"
												: "border-border bg-surface-primary hover:bg-surface-secondary/60",
										)}
										onClick={() => setSelectedProvider(providerState.provider)}
									>
										<div className="flex items-center justify-between gap-2">
											<div className="truncate text-sm font-medium text-content-primary">
												{providerState.label}
											</div>
											<Badge
												size="xs"
												variant={
													providerState.providerConfig
														? "green"
														: providerState.isEnvPreset
															? "info"
															: "default"
												}
											>
												{providerStatus}
											</Badge>
										</div>
										<div className="mt-1 truncate text-2xs text-content-secondary">
											{providerState.provider}
										</div>
										<div className="mt-1 truncate text-2xs text-content-secondary">
											{providerModelsLabel}
										</div>
									</button>
								);
							})}
						</div>
					</div>

					{selectedProviderState && (
						<div className="space-y-3 rounded-lg border border-border bg-surface-primary p-3">
							<div className="space-y-1">
								<div className="flex items-center gap-2 text-xs font-medium text-content-primary">
									2. Provider setup
									<Badge
										size="xs"
										variant={
											selectedProviderState.providerConfig
												? "green"
												: selectedProviderState.isEnvPreset
													? "info"
													: "default"
										}
									>
										{selectedProviderState.providerConfig
											? "Managed config"
											: selectedProviderState.isEnvPreset
												? "Environment preset"
												: "Not configured"}
									</Badge>
								</div>
								<div className="text-2xs text-content-secondary">
									{selectedProviderState.providerConfig
										? "Update this managed provider config for your deployment."
										: selectedProviderState.isEnvPreset
											? "This provider has an environment preset. Create a managed config to use BYO credentials and configure models."
											: "Create a managed provider config with BYO credentials before adding models."}
								</div>
							</div>

							{selectedProviderState.isEnvPreset &&
								!selectedProviderState.providerConfig && (
									<Alert severity="info">
										<AlertTitle>Environment preset detected.</AlertTitle>
										<AlertDetail>
											This provider is currently available through deployment
											environment settings. Create a managed config below if you
											want to override with BYO credentials.
										</AlertDetail>
									</Alert>
								)}

							<form className="space-y-2" onSubmit={handleSaveProviderConfig}>
								<div
									className={cn(
										"grid gap-2 sm:grid-cols-2",
										selectedProviderState.supportsBaseURL &&
											"lg:grid-cols-3",
									)}
								>
									<div className="grid gap-1 text-xs text-content-secondary">
										<label
											htmlFor={providerDisplayNameInputId}
											className="font-medium text-content-primary"
										>
											Display name (optional)
										</label>
										<Input
											id={providerDisplayNameInputId}
											className="h-9 text-xs"
											placeholder="Friendly provider label"
											value={providerDisplayName}
											onChange={(event) =>
												setProviderDisplayName(event.target.value)
											}
											disabled={providerConfigsUnavailable || isProviderMutationPending}
										/>
									</div>
									<div className="grid gap-1 text-xs text-content-secondary">
										<label
											htmlFor={providerAPIKeyInputId}
											className="font-medium text-content-primary"
										>
											{selectedProviderState.providerConfig
												? "API key (optional)"
												: "API key"}
										</label>
										<Input
											id={providerAPIKeyInputId}
											type="password"
											autoComplete="off"
											className="h-9 text-xs"
											placeholder={
												selectedProviderState.providerConfig
													? "Leave blank to keep existing key"
													: "Paste provider API key"
											}
											value={providerAPIKey}
											onChange={(event) => setProviderAPIKey(event.target.value)}
											disabled={providerConfigsUnavailable || isProviderMutationPending}
										/>
									</div>
									{selectedProviderState.supportsBaseURL && (
										<div className="grid gap-1 text-xs text-content-secondary">
											<label
												htmlFor={providerBaseURLInputId}
												className="font-medium text-content-primary"
											>
												Base URL (optional)
											</label>
											<Input
												id={providerBaseURLInputId}
												className="h-9 text-xs"
												placeholder="https://api.example.com/v1"
												value={providerBaseURL}
												onChange={(event) =>
													setProviderBaseURL(event.target.value)
												}
												disabled={
													providerConfigsUnavailable || isProviderMutationPending
												}
											/>
										</div>
									)}
								</div>
								<div className="flex items-center justify-between gap-2">
									<div className="text-2xs text-content-secondary">
										{selectedProviderState.providerConfig
											? "Updating the API key is optional."
											: "API key is required to create a managed provider config."}
									</div>
									<Button
										size="sm"
										type="submit"
										disabled={!canSaveProviderConfig}
									>
										{isProviderMutationPending && (
											<Loader2Icon className="h-4 w-4 animate-spin" />
										)}
										{selectedProviderState.providerConfig
											? "Save provider changes"
											: "Create provider config"}
									</Button>
								</div>
							</form>
						</div>
					)}

					<div className="space-y-3 rounded-lg border border-border bg-surface-primary p-3">
						<div className="space-y-1">
							<div className="text-xs font-medium text-content-primary">
								3. Models
							</div>
							<div className="text-2xs text-content-secondary">
								Add or remove models that end users can pick in Agents.
							</div>
						</div>
						{selectedProviderState && !modelConfigsUnavailable ? (
							canManageSelectedProviderModels ? (
								<form
									className="grid gap-2 md:grid-cols-[1fr_1fr_auto] md:items-end"
									onSubmit={(event) => void handleAddModel(event)}
								>
									<div className="grid gap-1 text-xs text-content-secondary">
										<label
											htmlFor={modelInputId}
											className="font-medium text-content-primary"
										>
											Model ID
										</label>
										<Input
											id={modelInputId}
											className="h-9 text-xs"
											placeholder="gpt-5, claude-sonnet-4-5, etc."
											value={model}
											onChange={(event) => setModel(event.target.value)}
											disabled={createModelConfigMutation.isPending}
										/>
									</div>
									<div className="grid gap-1 text-xs text-content-secondary">
										<label
											htmlFor={displayNameInputId}
											className="font-medium text-content-primary"
										>
											Display name (optional)
										</label>
										<Input
											id={displayNameInputId}
											className="h-9 text-xs"
											placeholder="Friendly label"
											value={displayName}
											onChange={(event) => setDisplayName(event.target.value)}
											disabled={createModelConfigMutation.isPending}
										/>
									</div>
									<Button
										size="sm"
										type="submit"
										disabled={createModelConfigMutation.isPending || !model.trim()}
									>
										<PlusIcon className="h-3.5 w-3.5" />
										Add model
									</Button>
								</form>
								) : (
									<div className="rounded-md border border-dashed border-border bg-surface-secondary/30 p-3 text-xs text-content-secondary">
										{!selectedProviderState.providerConfig
											? "Create a managed provider config before adding models."
											: "Set an API key for this provider before adding models."}
									</div>
								)
						) : (
							<div className="rounded-md border border-dashed border-border bg-surface-secondary/30 p-3 text-xs text-content-secondary">
								Select a provider to manage models.
							</div>
						)}

						<div className="space-y-2">
							<div className="text-xs font-medium text-content-primary">
								Configured models
							</div>
							{selectedProviderModels.length === 0 ? (
								<div className="rounded-md border border-dashed border-border bg-surface-secondary/30 p-3 text-xs text-content-secondary">
									No models configured for this provider.
								</div>
							) : (
								selectedProviderModels.map((modelConfig) => (
									<div
										key={modelConfig.id}
										className="flex items-start justify-between gap-2 rounded-md border border-border bg-surface-secondary/20 px-3 py-2"
									>
										<div className="min-w-0">
											<div className="truncate text-xs text-content-primary">
												{modelConfig.display_name || modelConfig.model}
											</div>
											<div className="truncate text-2xs text-content-secondary">
												{modelConfig.model}
												{modelConfig.enabled === false ? " . disabled" : ""}
												{modelConfig.is_default ? " . default" : ""}
											</div>
										</div>
										<Button
											size="xs"
											variant="destructive"
											onClick={() => void handleDeleteModel(modelConfig.id)}
											disabled={deleteModelConfigMutation.isPending}
										>
											<Trash2Icon />
											<span className="sr-only">Remove model</span>
										</Button>
									</div>
								))
							)}
						</div>
					</div>
				</div>
			)}
		</div>
	);
};
