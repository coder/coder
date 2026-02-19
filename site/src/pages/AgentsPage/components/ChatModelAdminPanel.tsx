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
import {
	Collapsible,
	CollapsibleContent,
	CollapsibleTrigger,
} from "components/Collapsible/Collapsible";
import { Input } from "components/Input/Input";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "components/Select/Select";
import { ExternalImage } from "components/ExternalImage/ExternalImage";
import { ChevronRightIcon, Loader2Icon, PlusIcon, ServerIcon, Trash2Icon } from "lucide-react";
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
	baseURL: string;
};

type ProviderConfigSource = "database" | "env_preset" | "supported";

export type ChatModelAdminSection = "providers" | "models";

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


const getProviderModelsLabel = (providerState: ProviderState): string => {
	if (providerState.modelConfigs.length > 0) {
		return `${providerState.modelConfigs.length} configured model${providerState.modelConfigs.length === 1 ? "" : "s"}`;
	}
	if (providerState.catalogModelCount > 0) {
		return `${providerState.catalogModelCount} catalog model${providerState.catalogModelCount === 1 ? "" : "s"}`;
	}
	return "No models configured";
};

const getProviderInputID = (baseID: string, provider: string): string => {
	const providerSlug = provider.replace(/[^a-zA-Z0-9_-]/g, "-");
	return `${baseID}-${providerSlug}`;
};

const isProviderAPIKeyEnvManaged = (
	providerState: ProviderState | null | undefined,
): boolean => {
	return Boolean(providerState?.isEnvPreset && !providerState.providerConfig);
};

const providerIconMap: Record<string, string> = {
	openai: "/icon/openai.svg",
	anthropic: "/icon/claude.svg",
	azure: "/icon/azure.svg",
	bedrock: "/icon/aws.svg",
	google: "/icon/google.svg",
	gemini: "/icon/gemini.svg",
};

// Some provider SVGs (e.g. OpenAI) are pure black and need
// inversion in dark mode to remain visible.
const darkInvertProviders = new Set(["openai"]);

const ProviderIcon: FC<{
	provider: string;
	className?: string;
	active?: boolean;
}> = ({ provider, className, active }) => {
	const normalized = normalizeProvider(provider);
	const iconPath = providerIconMap[normalized];
	if (iconPath) {
		return (
			<ExternalImage
				src={iconPath}
				alt={`${formatProviderLabel(provider)} logo`}
				className={cn(
					"shrink-0",
					!active && "grayscale opacity-50",
					darkInvertProviders.has(normalized) && "dark:invert",
					className,
				)}
			/>
		);
	}
	return (
		<ServerIcon
			className={cn(
				"shrink-0",
				active ? "text-content-primary" : "text-content-secondary",
				className,
			)}
		/>
	);
};

type ChatModelAdminPanelProps = {
	className?: string;
	section?: ChatModelAdminSection;
};

export const ChatModelAdminPanel: FC<ChatModelAdminPanelProps> = ({
	className,
	section = "providers",
}) => {
	const queryClient = useQueryClient();
	const [selectedProvider, setSelectedProvider] = useState<string | null>(null);
	const [expandedProvider, setExpandedProvider] = useState<string | null>(null);
	const [providerDisplayName, setProviderDisplayName] = useState("");
	const [providerAPIKey, setProviderAPIKey] = useState("");
	const [providerBaseURL, setProviderBaseURL] = useState("");
	const [model, setModel] = useState("");
	const [displayName, setDisplayName] = useState("");
	const [isAddModelOpen, setIsAddModelOpen] = useState(false);
	const providerDisplayNameInputId = useId();
	const providerAPIKeyInputId = useId();
	const providerBaseURLInputId = useId();
	const providerSelectInputId = useId();
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
		setExpandedProvider((current) => {
			if (
				current &&
				providerStates.some((providerState) => providerState.provider === current)
			) {
				return current;
			}
			return null;
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

	const expandedProviderState = useMemo(() => {
		if (!expandedProvider) {
			return null;
		}
		return (
			providerStates.find(
				(providerState) => providerState.provider === expandedProvider,
			) ?? null
		);
	}, [expandedProvider, providerStates]);

	useEffect(() => {
		if (!expandedProviderState) {
			setProviderDisplayName("");
			setProviderAPIKey("");
			setProviderBaseURL("");
			return;
		}
		setProviderDisplayName(
			readOptionalString(expandedProviderState.providerConfig?.display_name) ?? "",
		);
		setProviderAPIKey("");
		setProviderBaseURL(expandedProviderState.baseURL);
	}, [expandedProviderState]);

	const allConfiguredModels = modelConfigs;
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
	const requiresProviderAPIKey = Boolean(
		expandedProviderState &&
			!expandedProviderState.providerConfig &&
			!isProviderAPIKeyEnvManaged(expandedProviderState),
	);
	const canSaveProviderConfig = Boolean(
		expandedProviderState &&
			!providerConfigsUnavailable &&
			!isProviderMutationPending &&
			!isProviderAPIKeyEnvManaged(expandedProviderState) &&
			(!requiresProviderAPIKey || providerAPIKey.trim()),
	);
	const canManageSelectedProviderModels = Boolean(
		selectedProviderState?.providerConfig &&
			selectedProviderState.hasEffectiveAPIKey,
	);

	const handleSaveProviderConfig = async (event: FormEvent) => {
		event.preventDefault();
		if (
			!expandedProviderState ||
			providerConfigsUnavailable ||
			isProviderMutationPending ||
			isProviderAPIKeyEnvManaged(expandedProviderState)
		) {
			return;
		}

		const trimmedDisplayName = providerDisplayName.trim();
		const trimmedAPIKey = providerAPIKey.trim();
		const trimmedBaseURL = providerBaseURL.trim();

		if (expandedProviderState.providerConfig) {
			const currentDisplayName =
				readOptionalString(expandedProviderState.providerConfig.display_name) ?? "";
			const currentBaseURL = expandedProviderState.baseURL.trim();
			const req: UpdateChatProviderConfigRequest = {};

			if (trimmedDisplayName !== currentDisplayName) {
				req.display_name = trimmedDisplayName;
			}
			if (trimmedAPIKey) {
				req.api_key = trimmedAPIKey;
			}
			if (trimmedBaseURL !== currentBaseURL) {
				req.base_url = trimmedBaseURL;
			}
			if (Object.keys(req).length === 0) {
				return;
			}

			await updateProviderConfigMutation.mutateAsync({
				providerConfigId: expandedProviderState.providerConfig.id,
				req,
			});
		} else {
			if (!trimmedAPIKey) {
				return;
			}

			const req: CreateChatProviderConfigRequest = {
				provider: expandedProviderState.provider,
				api_key: trimmedAPIKey,
			};
			if (trimmedDisplayName) {
				req.display_name = trimmedDisplayName;
			}
			if (trimmedBaseURL) {
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
		setIsAddModelOpen(false);
	};

	const handleDeleteModel = async (modelConfigId: string) => {
		if (deleteModelConfigMutation.isPending) {
			return;
		}
		await deleteModelConfigMutation.mutateAsync(modelConfigId);
	};

	const renderHeader = () => {
		return (
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
		);
	};

	const renderAlerts = () => {
		return (
			<>
				{providerConfigsQuery.isError && (
					<ErrorAlert error={providerConfigsQuery.error} />
				)}
				{modelConfigsQuery.isError && <ErrorAlert error={modelConfigsQuery.error} />}
				{modelCatalogQuery.isError && <ErrorAlert error={modelCatalogQuery.error} />}
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
			</>
		);
	};

	const renderProviderForm = (providerState: ProviderState) => {
		const isExpanded = expandedProviderState?.provider === providerState.provider;
		if (!isExpanded) {
			return null;
		}

		const providerDisplayNameID = getProviderInputID(
			providerDisplayNameInputId,
			providerState.provider,
		);
		const providerAPIKeyID = getProviderInputID(
			providerAPIKeyInputId,
			providerState.provider,
		);
		const providerBaseURLID = getProviderInputID(
			providerBaseURLInputId,
			providerState.provider,
		);
		const isAPIKeyEnvManaged = isProviderAPIKeyEnvManaged(providerState);

		return (
			<CollapsibleContent className="border-t border-border px-5 py-4">
				<div className="space-y-3">
					<p className="m-0 text-xs text-content-secondary">
						{providerState.providerConfig
							? "Update this managed provider config for your deployment."
							: isAPIKeyEnvManaged
								? "This provider API key is managed by an environment variable."
								: "Create a managed provider config before enabling models."}
					</p>

					{isAPIKeyEnvManaged && (
						<Alert severity="info">
							<AlertTitle>API key managed by environment variable.</AlertTitle>
							<AlertDetail>
								This provider key is configured from deployment environment
								settings and cannot be edited in this UI.
							</AlertDetail>
						</Alert>
					)}

					{!isAPIKeyEnvManaged && (
						<form
							className="space-y-3"
							onSubmit={(event) => void handleSaveProviderConfig(event)}
						>
							<div className="grid gap-3 lg:grid-cols-3">
								<div className="grid gap-1.5">
									<label
										htmlFor={providerDisplayNameID}
										className="text-[13px] font-medium text-content-primary"
									>
										Display name{" "}
										<span className="font-normal text-content-secondary">(optional)</span>
									</label>
									<Input
										id={providerDisplayNameID}
										className="h-10 text-[13px]"
										placeholder="Friendly provider label"
										value={providerDisplayName}
										onChange={(event) =>
											setProviderDisplayName(event.target.value)
										}
										disabled={providerConfigsUnavailable || isProviderMutationPending}
									/>
								</div>
								<div className="grid gap-1.5">
									<label
										htmlFor={providerAPIKeyID}
										className="text-[13px] font-medium text-content-primary"
									>
										API key{" "}
										{providerState.providerConfig && (
											<span className="font-normal text-content-secondary">(optional)</span>
										)}
									</label>
									<Input
										id={providerAPIKeyID}
										type="password"
										autoComplete="off"
										className="h-10 text-[13px]"
										placeholder={
											providerState.providerConfig
												? "Leave blank to keep existing key"
												: "Paste provider API key"
										}
										value={providerAPIKey}
										onChange={(event) => setProviderAPIKey(event.target.value)}
										disabled={providerConfigsUnavailable || isProviderMutationPending}
									/>
								</div>
								<div className="grid gap-1.5">
									<label
										htmlFor={providerBaseURLID}
										className="text-[13px] font-medium text-content-primary"
									>
										Base URL{" "}
										<span className="font-normal text-content-secondary">(optional)</span>
									</label>
									<Input
										id={providerBaseURLID}
										className="h-10 text-[13px]"
										placeholder="https://api.example.com/v1"
										value={providerBaseURL}
										onChange={(event) => setProviderBaseURL(event.target.value)}
										disabled={providerConfigsUnavailable || isProviderMutationPending}
									/>
								</div>
							</div>
							<div className="flex items-center justify-end gap-3 border-t border-border pt-3">
								<Button size="sm" type="submit" disabled={!canSaveProviderConfig}>
									{isProviderMutationPending && (
										<Loader2Icon className="h-4 w-4 animate-spin" />
									)}
									{providerState.providerConfig
										? "Save changes"
										: "Create provider config"}
								</Button>
							</div>
						</form>
					)}
				</div>
			</CollapsibleContent>
		);
	};

	const renderProvidersSection = () => {
		if (!hasProviderOptions) {
			return (
				<div className="rounded-lg border border-dashed border-border bg-surface-primary p-4 text-[13px] text-content-secondary">
					No provider types were returned by the backend.
				</div>
			);
		}

		return (
			<div className="space-y-2">
				{providerStates.map((providerState) => {
					const isExpanded = expandedProvider === providerState.provider;
					const providerModelsLabel = getProviderModelsLabel(providerState);

					return (
						<Collapsible
							key={providerState.provider}
							open={isExpanded}
							onOpenChange={(nextOpen) => {
								setExpandedProvider(nextOpen ? providerState.provider : null);
								if (nextOpen) {
									setSelectedProvider(providerState.provider);
								}
							}}
						>
							<div
								className={cn(
									"rounded-xl border border-border-default bg-surface-primary shadow-sm transition-all",
									isExpanded && "border-border-default bg-surface-secondary/30 shadow-md",
								)}
							>
								<CollapsibleTrigger asChild>
									<Button
										variant="subtle"
										className={cn(
											"h-auto w-full justify-between gap-4 rounded-[inherit] px-5 py-3.5 text-left shadow-none",
											isExpanded
												? "bg-surface-secondary/30 hover:bg-surface-secondary/30"
												: "hover:bg-surface-tertiary/30",
										)}
									>
										<div className="flex min-w-0 items-center gap-3">
											<ProviderIcon
												provider={providerState.provider}
												className="h-7 w-7"
												active={providerState.hasEffectiveAPIKey}
											/>
											<div className="min-w-0">
												<span className={cn(
													"truncate text-[15px] font-semibold",
													providerState.hasEffectiveAPIKey
														? "text-content-primary"
														: "text-content-secondary",
												)}>
													{providerState.label}
												</span>
												<div className="mt-0.5 flex items-center gap-2 text-xs text-content-secondary">
													<span className="truncate">{providerModelsLabel}</span>
												</div>
											</div>
										</div>
										<ChevronRightIcon
											className={cn(
												"h-4 w-4 shrink-0 text-content-secondary transition-transform duration-200",
												isExpanded && "rotate-90 text-content-primary",
											)}
										/>
									</Button>
								</CollapsibleTrigger>
								{renderProviderForm(providerState)}
							</div>
						</Collapsible>
					);
				})}
			</div>
		);
	};

	const renderAddModelForm = () => {
		if (!selectedProviderState || modelConfigsUnavailable) {
			return (
				<div className="space-y-3 px-4 pb-4 pt-3">
					<div className="grid gap-1.5">
						<label
							htmlFor={providerSelectInputId}
							className="text-[13px] font-medium text-content-primary"
						>
							Provider
						</label>
						<Select
							value={selectedProvider ?? ""}
							onValueChange={setSelectedProvider}
							disabled={!hasProviderOptions}
						>
							<SelectTrigger id={providerSelectInputId} className="h-10 max-w-[240px] text-[13px]">
								<SelectValue placeholder="Select provider" />
							</SelectTrigger>
							<SelectContent>
								{providerStates.map((providerState) => (
									<SelectItem
										key={providerState.provider}
										value={providerState.provider}
									>
										<span className="flex items-center gap-2">
											<ProviderIcon
												provider={providerState.provider}
												className="h-4 w-4"
											/>
											{providerState.label}
										</span>
									</SelectItem>
								))}
							</SelectContent>
						</Select>
					</div>
				</div>
			);
		}

		if (!canManageSelectedProviderModels) {
			return (
				<div className="space-y-3 px-4 pb-4 pt-3">
					<div className="grid gap-1.5">
						<label
							htmlFor={providerSelectInputId}
							className="text-[13px] font-medium text-content-primary"
						>
							Provider
						</label>
						<Select
							value={selectedProvider ?? ""}
							onValueChange={setSelectedProvider}
							disabled={!hasProviderOptions}
						>
							<SelectTrigger id={providerSelectInputId} className="h-10 max-w-[240px] text-[13px]">
								<SelectValue placeholder="Select provider" />
							</SelectTrigger>
							<SelectContent>
								{providerStates.map((providerState) => (
									<SelectItem
										key={providerState.provider}
										value={providerState.provider}
									>
										<span className="flex items-center gap-2">
											<ProviderIcon
												provider={providerState.provider}
												className="h-4 w-4"
											/>
											{providerState.label}
										</span>
									</SelectItem>
								))}
							</SelectContent>
						</Select>
					</div>
					<p className="text-[13px] text-content-secondary">
						{!selectedProviderState.providerConfig
							? "Create a managed provider config on the Providers tab before adding models."
							: "Set an API key for this provider on the Providers tab before adding models."}
					</p>
				</div>
			);
		}

		return (
			<form
				className="space-y-3 px-4 pb-4 pt-3"
				onSubmit={(event) => void handleAddModel(event)}
			>
				<div className="grid gap-3 md:grid-cols-3">
					<div className="grid gap-1.5">
						<label
							htmlFor={providerSelectInputId}
							className="text-[13px] font-medium text-content-primary"
						>
							Provider
						</label>
						<Select
							value={selectedProvider ?? ""}
							onValueChange={setSelectedProvider}
							disabled={!hasProviderOptions}
						>
							<SelectTrigger id={providerSelectInputId} className="h-10 text-[13px]">
								<SelectValue placeholder="Select provider" />
							</SelectTrigger>
							<SelectContent>
								{providerStates.map((providerState) => (
									<SelectItem
										key={providerState.provider}
										value={providerState.provider}
										disabled={!providerState.hasEffectiveAPIKey}
									>
										<span className="flex items-center gap-2">
											<ProviderIcon
												provider={providerState.provider}
												className="h-4 w-4"
											/>
											{providerState.label}
											{!providerState.hasEffectiveAPIKey && (
												<span className="text-xs text-content-disabled">
													(not configured)
												</span>
											)}
										</span>
									</SelectItem>
								))}
							</SelectContent>
						</Select>
					</div>
					<div className="grid gap-1.5">
						<label
							htmlFor={modelInputId}
							className="text-[13px] font-medium text-content-primary"
						>
							Model ID
						</label>
						<Input
							id={modelInputId}
							className="h-10 text-[13px]"
							placeholder="gpt-5, claude-sonnet-4-5, etc."
							value={model}
							onChange={(event) => setModel(event.target.value)}
							disabled={createModelConfigMutation.isPending}
						/>
					</div>
					<div className="grid gap-1.5">
						<label
							htmlFor={displayNameInputId}
							className="text-[13px] font-medium text-content-primary"
						>
							Display name{" "}
							<span className="font-normal text-content-secondary">(optional)</span>
						</label>
						<Input
							id={displayNameInputId}
							className="h-10 text-[13px]"
							placeholder="Friendly label"
							value={displayName}
							onChange={(event) => setDisplayName(event.target.value)}
							disabled={createModelConfigMutation.isPending}
						/>
					</div>
				</div>
				<div className="flex items-center justify-end gap-2">
					<Button
						size="sm"
						variant="outline"
						type="button"
						onClick={() => {
							setIsAddModelOpen(false);
							setModel("");
							setDisplayName("");
						}}
					>
						Cancel
					</Button>
					<Button
						size="sm"
						type="submit"
						disabled={createModelConfigMutation.isPending || !model.trim()}
					>
						{createModelConfigMutation.isPending ? (
							<Loader2Icon className="h-4 w-4 animate-spin" />
						) : (
							<PlusIcon className="h-4 w-4" />
						)}
						Add model
					</Button>
				</div>
			</form>
		);
	};

	const renderModelsSection = () => {
		return (
			<div className="space-y-4">
				{/* Model list */}
				<div className="overflow-hidden rounded-xl border border-border">
					{allConfiguredModels.length === 0 ? (
						<div className="px-4 py-8 text-center text-[13px] text-content-secondary">
							No models configured yet. Add one to get started.
						</div>
					) : (
						allConfiguredModels.map((modelConfig, index) => (
							<div
								key={modelConfig.id}
								className={cn(
									"group flex items-center justify-between gap-3 bg-surface-primary px-4 py-3 transition-colors hover:bg-surface-secondary/20",
									index > 0 && "border-t border-border",
								)}
							>
								<div className="flex min-w-0 items-center gap-3">
									<ProviderIcon
										provider={modelConfig.provider}
										className="h-5 w-5"
									/>
									<div className="min-w-0">
										<div className="truncate text-[13px] font-medium text-content-primary">
											{modelConfig.display_name || modelConfig.model}
										</div>
										<div className="mt-0.5 flex items-center gap-1.5 truncate text-xs text-content-secondary">
											<span>{formatProviderLabel(modelConfig.provider)}</span>
											<span className="text-content-disabled">&middot;</span>
											<span>{modelConfig.model}</span>
											{modelConfig.enabled === false && (
												<>
													<span className="text-content-disabled">&middot;</span>
													<span className="text-content-disabled">disabled</span>
												</>
											)}
											{modelConfig.is_default && (
												<>
													<span className="text-content-disabled">&middot;</span>
													<Badge size="sm" variant="info">default</Badge>
												</>
											)}
										</div>
									</div>
								</div>
								<Button
									size="icon"
									variant="subtle"
									className="h-8 w-8 shrink-0 text-content-secondary opacity-0 transition-opacity hover:text-content-primary group-hover:opacity-100"
									onClick={() => void handleDeleteModel(modelConfig.id)}
									disabled={deleteModelConfigMutation.isPending}
								>
									<Trash2Icon className="h-4 w-4" />
									<span className="sr-only">Remove model</span>
								</Button>
							</div>
						))
					)}

					{/* Add model trigger / inline form */}
					{isAddModelOpen ? (
						<div className="border-t border-border bg-surface-secondary/10">
							{renderAddModelForm()}
						</div>
					) : (
						<div className={cn(allConfiguredModels.length > 0 && "border-t border-border")}>
							<Button
								variant="subtle"
								className="h-auto w-full justify-start gap-2 rounded-none border-none px-4 py-3 text-[13px] font-medium text-content-link shadow-none hover:bg-surface-secondary/20"
								onClick={() => setIsAddModelOpen(true)}
							>
								<PlusIcon className="h-4 w-4" />
								Add model
							</Button>
						</div>
					)}
				</div>
			</div>
		);
	};

	return (
		<div className={cn("space-y-3", className)}>
			{renderHeader()}
			{renderAlerts()}
			{section === "providers" ? renderProvidersSection() : renderModelsSection()}
		</div>
	);
};
