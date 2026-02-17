import {
	type ChatModelConfig,
	type ChatModelsResponse,
	type ChatProviderConfig,
	type CreateChatModelConfigRequest,
} from "api/api";
import {
	chatModelConfigs,
	chatModels,
	chatProviderConfigs,
	createChatModelConfig as createChatModelConfigMutation,
	deleteChatModelConfig as deleteChatModelConfigMutation,
} from "api/queries/chats";
import { Alert, AlertDetail, AlertTitle } from "components/Alert/Alert";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Button } from "components/Button/Button";
import { Input } from "components/Input/Input";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "components/Select/Select";
import { Loader2Icon, Trash2Icon } from "lucide-react";
import { type FC, type FormEvent, useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { cn } from "utils/cn";

type ProviderKey = "openai" | "anthropic";

type ProviderDefinition = {
	key: ProviderKey;
	provider: string;
	label: string;
};

const providerDefinitions: readonly ProviderDefinition[] = [
	{ key: "openai", provider: "openai", label: "OpenAI" },
	{ key: "anthropic", provider: "anthropic", label: "Anthropic" },
];

const providerAliases: Record<ProviderKey, readonly string[]> = {
	openai: ["openai"],
	anthropic: ["anthropic"],
};

const normalizeProvider = (provider: string): string =>
	provider.trim().toLowerCase();

const getProviderKey = (provider: string): ProviderKey | null => {
	const normalized = normalizeProvider(provider);

	for (const definition of providerDefinitions) {
		if (providerAliases[definition.key].includes(normalized)) {
			return definition.key;
		}
	}

	return null;
};

const getProviderDefinition = (providerKey: ProviderKey): ProviderDefinition => {
	const providerDefinition = providerDefinitions.find(
		(definition) => definition.key === providerKey,
	);
	if (!providerDefinition) {
		throw new Error(`Unsupported provider key: ${providerKey}`);
	}
	return providerDefinition;
};

const hasProviderAPIKey = (providerConfig: ChatProviderConfig | undefined) => {
	if (!providerConfig) {
		return false;
	}
	const providerConfigWithLegacyAPIKeyField = providerConfig as ChatProviderConfig & {
		has_api_key?: boolean;
	};
	return Boolean(
		providerConfig.api_key_set ?? providerConfigWithLegacyAPIKeyField.has_api_key,
	);
};

type CatalogProvider = ChatModelsResponse["providers"][number];

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

type ChatModelAdminPanelProps = {
	className?: string;
};

export const ChatModelAdminPanel: FC<ChatModelAdminPanelProps> = ({
	className,
}) => {
	const queryClient = useQueryClient();
	const [selectedProviderKey, setSelectedProviderKey] =
		useState<ProviderKey | null>(null);
	const [model, setModel] = useState("");
	const [displayName, setDisplayName] = useState("");

	const providerConfigsQuery = useQuery(chatProviderConfigs());
	const modelConfigsQuery = useQuery(chatModelConfigs());
	const modelCatalogQuery = useQuery(chatModels());

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

	const modelConfigsByProvider = useMemo<
		Record<ProviderKey, readonly ChatModelConfig[]>
	>(() => {
		const configsByProvider: Record<ProviderKey, ChatModelConfig[]> = {
			openai: [],
			anthropic: [],
		};

		for (const modelConfig of modelConfigs) {
			const providerKey = getProviderKey(modelConfig.provider);
			if (!providerKey) {
				continue;
			}
			configsByProvider[providerKey].push(modelConfig);
		}

		return configsByProvider;
	}, [modelConfigs]);

	const unsupportedModelConfigs = useMemo(
		() =>
			modelConfigs.filter(
				(modelConfig) => getProviderKey(modelConfig.provider) === null,
			),
		[modelConfigs],
	);

	const providerOptions = useMemo(() => {
		const configuredProviders = new Set<ProviderKey>();

		for (const providerConfig of providerConfigsQuery.data ?? []) {
			const providerKey = getProviderKey(providerConfig.provider);
			if (!providerKey || !hasProviderAPIKey(providerConfig)) {
				continue;
			}
			configuredProviders.add(providerKey);
		}

		for (const provider of getCatalogProviders(modelCatalogQuery.data)) {
			const providerKey = getProviderKey(provider.provider);
			if (!providerKey || !providerHasCatalogAPIKey(provider)) {
				continue;
			}
			configuredProviders.add(providerKey);
		}

		return providerDefinitions.filter((definition) =>
			configuredProviders.has(definition.key),
		);
	}, [providerConfigsQuery.data, modelCatalogQuery.data]);

	useEffect(() => {
		setSelectedProviderKey((current) => {
			if (current && providerOptions.some((option) => option.key === current)) {
				return current;
			}
			return providerOptions[0]?.key ?? null;
		});
	}, [providerOptions]);

	const selectedProviderModels = selectedProviderKey
		? modelConfigsByProvider[selectedProviderKey]
		: [];
	const hasConfiguredProviders = providerOptions.length > 0;
	const modelMutationError =
		createModelConfigMutation.error ?? deleteModelConfigMutation.error;
	const isLoading =
		providerConfigsQuery.isLoading ||
		modelConfigsQuery.isLoading ||
		modelCatalogQuery.isLoading;
	const modelConfigsUnavailable = modelConfigsQuery.data === null;

	const handleAddModel = async (event: FormEvent) => {
		event.preventDefault();
		if (!selectedProviderKey || createModelConfigMutation.isPending) {
			return;
		}

		const trimmedModel = model.trim();
		if (!trimmedModel) {
			return;
		}

		const providerDefinition = getProviderDefinition(selectedProviderKey);
		const req: CreateChatModelConfigRequest = {
			provider: providerDefinition.provider,
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
						Select provider, then add or remove models.
					</div>
				</div>
				{isLoading && (
					<div className="flex items-center gap-1 text-2xs text-content-secondary">
						<Loader2Icon className="h-3.5 w-3.5 animate-spin" />
						Loading
					</div>
				)}
			</div>

			{providerConfigsQuery.isError && <ErrorAlert error={providerConfigsQuery.error} />}
			{modelConfigsQuery.isError && <ErrorAlert error={modelConfigsQuery.error} />}
			{modelCatalogQuery.isError && <ErrorAlert error={modelCatalogQuery.error} />}
			{modelMutationError && <ErrorAlert error={modelMutationError} />}

			{modelConfigsUnavailable && (
				<Alert severity="info" className="mb-3">
					<AlertTitle>Chat model admin API is unavailable on this deployment.</AlertTitle>
					<AlertDetail>/api/v2/chats/model-configs is missing.</AlertDetail>
				</Alert>
			)}

			{unsupportedModelConfigs.length > 0 && (
				<Alert severity="info" className="mb-3">
					<AlertTitle>Some model configs are outside this simplified UI.</AlertTitle>
					<AlertDetail>
						This panel only edits OpenAI and Anthropic model configs.
					</AlertDetail>
				</Alert>
			)}

			{!hasConfiguredProviders ? (
				<div className="rounded-md border border-dashed border-border bg-surface-primary p-3 text-xs text-content-secondary">
					No OpenAI or Anthropic providers with API keys are configured.
				</div>
			) : (
				<div className="space-y-4">
					<div className="grid gap-1.5">
						<div className="text-xs font-medium text-content-primary">Provider</div>
						<Select
							value={selectedProviderKey ?? undefined}
							onValueChange={(value) => setSelectedProviderKey(value as ProviderKey)}
							disabled={modelConfigsUnavailable}
						>
							<SelectTrigger className="h-9 max-w-xs text-xs">
								<SelectValue placeholder="Select provider" />
							</SelectTrigger>
							<SelectContent>
								{providerOptions.map((provider) => (
									<SelectItem key={provider.key} value={provider.key}>
										{provider.label}
									</SelectItem>
								))}
							</SelectContent>
						</Select>
					</div>

					{selectedProviderKey && !modelConfigsUnavailable && (
						<form
							className="grid gap-2 md:grid-cols-[1fr_1fr_auto] md:items-end"
							onSubmit={(event) => void handleAddModel(event)}
						>
							<label className="grid gap-1 text-xs text-content-secondary">
								<span className="font-medium text-content-primary">Model ID</span>
								<Input
									className="h-9 text-xs"
									placeholder="gpt-5, claude-sonnet-4-5, etc."
									value={model}
									onChange={(event) => setModel(event.target.value)}
									disabled={createModelConfigMutation.isPending}
								/>
							</label>
							<label className="grid gap-1 text-xs text-content-secondary">
								<span className="font-medium text-content-primary">
									Display name (optional)
								</span>
								<Input
									className="h-9 text-xs"
									placeholder="Friendly label"
									value={displayName}
									onChange={(event) => setDisplayName(event.target.value)}
									disabled={createModelConfigMutation.isPending}
								/>
							</label>
							<Button
								size="sm"
								type="submit"
								disabled={createModelConfigMutation.isPending || !model.trim()}
							>
								Add model
							</Button>
						</form>
					)}

					<div className="space-y-2">
						<div className="text-xs font-medium text-content-primary">
							Configured models
						</div>
						{selectedProviderModels.length === 0 ? (
							<div className="rounded-md border border-dashed border-border bg-surface-primary p-3 text-xs text-content-secondary">
								No models configured for this provider.
							</div>
						) : (
							selectedProviderModels.map((modelConfig) => (
								<div
									key={modelConfig.id}
									className="flex items-start justify-between gap-2 rounded-md border border-border bg-surface-primary px-3 py-2"
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
			)}
		</div>
	);
};
