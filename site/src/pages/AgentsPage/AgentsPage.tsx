import { watchChats } from "api/api";
import { getErrorMessage } from "api/errors";
import {
	chatKey,
	chatModelConfigs,
	chatModels,
	chats,
	chatsKey,
	createChat,
	deleteChat,
} from "api/queries/chats";
import { workspaces } from "api/queries/workspaces";
import type * as TypesGen from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import type { ModelSelectorOption } from "components/ai-elements";
import { Button } from "components/Button/Button";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "components/Select/Select";
import { useAuthenticated } from "hooks";
import { MonitorIcon } from "lucide-react";
import { UserDropdown } from "modules/dashboard/Navbar/UserDropdown/UserDropdown";
import { useDashboard } from "modules/dashboard/useDashboard";
import {
	type FC,
	useCallback,
	useEffect,
	useMemo,
	useRef,
	useState,
} from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { Outlet, useNavigate, useParams } from "react-router";
import { toast } from "sonner";
import { cn } from "utils/cn";
import { pageTitle } from "utils/page";
import { AgentChatInput } from "./AgentChatInput";
import { AgentsSidebar } from "./AgentsSidebar";
import { ConfigureAgentsDialog } from "./ConfigureAgentsDialog";
import {
	getModelCatalogStatusMessage,
	getModelOptionsFromCatalog,
	getModelSelectorPlaceholder,
	hasConfiguredModelsInCatalog,
} from "./modelOptions";

const emptyInputStorageKey = "agents.empty-input";
const selectedWorkspaceIdStorageKey = "agents.selected-workspace-id";
const lastModelConfigIDStorageKey = "agents.last-model-config-id";
const nilUUID = "00000000-0000-0000-0000-000000000000";

type ChatModelOption = ModelSelectorOption;

type CreateChatOptions = {
	message: string;
	workspaceId?: string;
	model?: string;
};

// Type guard for SSE events from the chat list watch endpoint.
function isChatListSSEEvent(
	data: unknown,
): data is { kind: string; chat: TypesGen.Chat } {
	if (typeof data !== "object" || data === null) return false;
	const obj = data as Record<string, unknown>;
	return (
		typeof obj.kind === "string" &&
		typeof obj.chat === "object" &&
		obj.chat !== null &&
		"id" in obj.chat
	);
}

export interface AgentsOutletContext {
	chatErrorReasons: Record<string, string>;
	setChatErrorReason: (chatId: string, reason: string) => void;
	clearChatErrorReason: (chatId: string) => void;
	requestArchiveAgent: (chatId: string) => void;
}

export const AgentsPage: FC = () => {
	const queryClient = useQueryClient();
	const navigate = useNavigate();
	const { agentId } = useParams();
	const { permissions, user, signOut } = useAuthenticated();
	const { appearance, buildInfo } = useDashboard();
	const isAgentsAdmin =
		permissions.editDeploymentConfig ||
		user.roles.some((role) => role.name === "owner" || role.name === "admin");
	const canSetSystemPrompt = isAgentsAdmin;
	const hasAdminControls = canSetSystemPrompt || isAgentsAdmin;
	const [isConfigureAgentsDialogOpen, setConfigureAgentsDialogOpen] =
		useState(false);

	const chatsQuery = useQuery(chats());
	const chatModelsQuery = useQuery(chatModels());
	const chatModelConfigsQuery = useQuery(chatModelConfigs());
	const createMutation = useMutation(createChat(queryClient));
	const archiveMutation = useMutation(deleteChat(queryClient));
	const [archivingChatId, setArchivingChatId] = useState<string | null>(null);
	const [chatErrorReasons, setChatErrorReasons] = useState<
		Record<string, string>
	>({});
	const catalogModelOptions = useMemo(
		() =>
			getModelOptionsFromCatalog(
				chatModelsQuery.data,
				chatModelConfigsQuery.data,
			),
		[chatModelsQuery.data, chatModelConfigsQuery.data],
	);
	const modelConfigIDByModelID = useMemo(() => {
		const byModelID = new Map<string, string>();
		for (const config of chatModelConfigsQuery.data ?? []) {
			const provider = config.provider.trim().toLowerCase();
			const model = config.model.trim();
			if (!provider || !model) {
				continue;
			}
			const colonRef = `${provider}:${model}`;
			if (!byModelID.has(colonRef)) {
				byModelID.set(colonRef, config.id);
			}
			const slashRef = `${provider}/${model}`;
			if (!byModelID.has(slashRef)) {
				byModelID.set(slashRef, config.id);
			}
		}
		return byModelID;
	}, [chatModelConfigsQuery.data]);
	const setChatErrorReason = useCallback((chatId: string, reason: string) => {
		const trimmedReason = reason.trim();
		if (!chatId || !trimmedReason) {
			return;
		}
		setChatErrorReasons((current) => {
			if (current[chatId] === trimmedReason) {
				return current;
			}
			return {
				...current,
				[chatId]: trimmedReason,
			};
		});
	}, []);
	const clearChatErrorReason = useCallback((chatId: string) => {
		if (!chatId) {
			return;
		}
		setChatErrorReasons((current) => {
			if (!(chatId in current)) {
				return current;
			}
			const next = { ...current };
			delete next[chatId];
			return next;
		});
	}, []);
	const chatList = chatsQuery.data ?? [];
	const requestArchiveAgent = useCallback(
		async (chatId: string) => {
			if (archiveMutation.isPending) {
				return;
			}

			setArchivingChatId(chatId);
			const nextChatId = (
				queryClient.getQueryData(chats().queryKey) as
					| TypesGen.Chat[]
					| undefined
			)?.find((chat) => chat.id !== chatId)?.id;

			try {
				await archiveMutation.mutateAsync(chatId);
				clearChatErrorReason(chatId);
				toast.success("Agent archived.");

				if (chatId === agentId) {
					navigate(nextChatId ? `/agents/${nextChatId}` : "/agents", {
						replace: true,
					});
				}
			} catch (error) {
				toast.error(getErrorMessage(error, "Failed to archive agent."));
			} finally {
				setArchivingChatId(null);
			}
		},
		[archiveMutation, queryClient, agentId, navigate, clearChatErrorReason],
	);
	const outletContext: AgentsOutletContext = useMemo(
		() => ({
			chatErrorReasons,
			setChatErrorReason,
			clearChatErrorReason,
			requestArchiveAgent,
		}),
		[
			chatErrorReasons,
			setChatErrorReason,
			clearChatErrorReason,
			requestArchiveAgent,
		],
	);
	const handleCreateChat = async (options: CreateChatOptions) => {
		const { message, workspaceId, model } = options;
		const modelConfigID =
			(model && modelConfigIDByModelID.get(model)) || nilUUID;
		const createdChat = await createMutation.mutateAsync({
			content: [{ type: "text", text: message }],
			workspace_id: workspaceId,
			model_config_id: modelConfigID,
		});

		if (typeof window !== "undefined") {
			localStorage.removeItem(emptyInputStorageKey);
			if (modelConfigID !== nilUUID) {
				localStorage.setItem(lastModelConfigIDStorageKey, modelConfigID);
			} else {
				localStorage.removeItem(lastModelConfigIDStorageKey);
			}
		}

		navigate(`/agents/${createdChat.id}`);
	};

	const handleNewAgent = () => {
		if (typeof window !== "undefined") {
			localStorage.setItem(emptyInputStorageKey, "");
		}
		navigate("/agents");
	};

	useEffect(() => {
		const ws = watchChats();
		ws.addEventListener("message", (event) => {
			const sse = event.parsedMessage;
			if (sse?.type !== "data" || !sse.data) {
				return;
			}
			if (!isChatListSSEEvent(sse.data)) {
				return;
			}
			const chatEvent = sse.data;
			const updatedChat = chatEvent.chat;

			if (chatEvent.kind === "deleted") {
				queryClient.setQueryData(
					chatsKey,
					(prev: TypesGen.Chat[] | undefined) =>
						prev?.filter((c) => c.id !== updatedChat.id),
				);
				queryClient.removeQueries({
					queryKey: chatKey(updatedChat.id),
					exact: true,
				});
				return;
			}

			queryClient.setQueryData(
				chatsKey,
				(prev: TypesGen.Chat[] | undefined) => {
					if (!prev) return prev;
					const exists = prev.some((c) => c.id === updatedChat.id);
					if (exists) {
						return prev.map((c) =>
							c.id === updatedChat.id
								? {
										...c,
										status: updatedChat.status,
										title: updatedChat.title,
										updated_at: updatedChat.updated_at,
									}
								: c,
						);
					}
					if (chatEvent.kind === "created") {
						return [updatedChat, ...prev];
					}
					return prev;
				},
			);
			queryClient.setQueryData<TypesGen.ChatWithMessages | undefined>(
				chatKey(updatedChat.id),
				(previousChat) => {
					if (!previousChat) {
						return previousChat;
					}
					return {
						...previousChat,
						chat: {
							...previousChat.chat,
							status: updatedChat.status,
							title: updatedChat.title,
							updated_at: updatedChat.updated_at,
						},
					};
				},
			);
		});
		return () => ws.close();
	}, [queryClient]);

	useEffect(() => {
		document.title = pageTitle("Agents");
	}, []);

	return (
		<div className="flex h-full min-h-0 flex-col overflow-hidden bg-surface-primary md:flex-row">
			<div
				className={cn(
					"shrink-0 h-[42dvh] min-h-[240px] border-b border-border-default md:h-full md:w-[320px] md:min-h-0 md:border-b-0",
					agentId && "hidden md:block",
				)}
			>
				<AgentsSidebar
					chats={chatList}
					chatErrorReasons={chatErrorReasons}
					modelOptions={catalogModelOptions}
					modelConfigs={chatModelConfigsQuery.data ?? []}
					logoUrl={appearance.logo_url}
					onArchiveAgent={requestArchiveAgent}
					onNewAgent={handleNewAgent}
					isCreating={createMutation.isPending}
					isArchiving={archiveMutation.isPending}
					archivingChatId={archivingChatId}
					isLoading={chatsQuery.isLoading}
					loadError={chatsQuery.isError ? chatsQuery.error : undefined}
					onRetryLoad={() => void chatsQuery.refetch()}
				/>
			</div>

			{agentId ? (
				<Outlet context={outletContext} />
			) : (
				<div className="flex min-h-0 min-w-0 flex-1 flex-col bg-surface-primary">
					<div className="flex shrink-0 items-center gap-2 px-4 py-0.5">
						<div className="flex-1" />
						{hasAdminControls && (
							<Button
								variant="subtle"
								disabled={createMutation.isPending}
								className="h-8 gap-1.5 border-none bg-transparent px-1 text-[13px] shadow-none hover:bg-transparent"
								onClick={() => setConfigureAgentsDialogOpen(true)}
							>
								Admin
							</Button>
						)}
						<div className="flex items-center [&_span]:!rounded-full [&_span]:!size-8 [&_span]:!text-xs">
							<UserDropdown
								user={user}
								buildInfo={buildInfo}
								supportLinks={
									appearance.support_links?.filter(
										(link) => link.location !== "navbar",
									) ?? []
								}
								onSignOut={signOut}
							/>
						</div>
					</div>
					<AgentsEmptyState
						onCreateChat={handleCreateChat}
						isCreating={createMutation.isPending}
						createError={createMutation.error}
						modelCatalog={chatModelsQuery.data}
						modelOptions={catalogModelOptions}
						modelConfigs={chatModelConfigsQuery.data ?? []}
						isModelCatalogLoading={chatModelsQuery.isLoading}
						isModelConfigsLoading={chatModelConfigsQuery.isLoading}
						modelCatalogError={chatModelsQuery.error}
					/>
				</div>
			)}

			{hasAdminControls && (
				<ConfigureAgentsDialog
					open={isConfigureAgentsDialogOpen}
					onOpenChange={setConfigureAgentsDialogOpen}
					canManageChatModelConfigs={isAgentsAdmin}
					canSetSystemPrompt={canSetSystemPrompt}
				/>
			)}
		</div>
	);
};

interface AgentsEmptyStateProps {
	onCreateChat: (options: CreateChatOptions) => Promise<void>;
	isCreating: boolean;
	createError: unknown;
	modelCatalog: TypesGen.ChatModelsResponse | null | undefined;
	modelOptions: readonly ChatModelOption[];
	isModelCatalogLoading: boolean;
	modelConfigs: readonly TypesGen.ChatModelConfig[];
	isModelConfigsLoading: boolean;
	modelCatalogError: unknown;
}

export const AgentsEmptyState: FC<AgentsEmptyStateProps> = ({
	onCreateChat,
	isCreating,
	createError,
	modelCatalog,
	modelOptions,
	modelConfigs,
	isModelCatalogLoading,
	isModelConfigsLoading,
	modelCatalogError,
}) => {
	const initialInput = useMemo(() => {
		if (typeof window === "undefined") {
			return "";
		}
		return localStorage.getItem(emptyInputStorageKey) ?? "";
	}, []);
	const initialLastModelConfigID = useMemo(() => {
		if (typeof window === "undefined") {
			return "";
		}
		return localStorage.getItem(lastModelConfigIDStorageKey) ?? "";
	}, []);
	const modelIDByConfigID = useMemo(() => {
		const optionIDByRef = new Map<string, string>();
		for (const option of modelOptions) {
			const provider = option.provider.trim().toLowerCase();
			const model = option.model.trim();
			if (!provider || !model) {
				continue;
			}
			const key = `${provider}:${model}`;
			if (!optionIDByRef.has(key)) {
				optionIDByRef.set(key, option.id);
			}
		}

		const byConfigID = new Map<string, string>();
		for (const config of modelConfigs) {
			const provider = config.provider.trim().toLowerCase();
			const model = config.model.trim();
			if (!provider || !model) {
				continue;
			}
			const modelID = optionIDByRef.get(`${provider}:${model}`);
			if (!modelID || byConfigID.has(config.id)) {
				continue;
			}
			byConfigID.set(config.id, modelID);
		}
		return byConfigID;
	}, [modelConfigs, modelOptions]);
	const lastUsedModelID = useMemo(() => {
		if (!initialLastModelConfigID) {
			return "";
		}
		return modelIDByConfigID.get(initialLastModelConfigID) ?? "";
	}, [initialLastModelConfigID, modelIDByConfigID]);
	const defaultModelID = useMemo(() => {
		const defaultModelConfig = modelConfigs.find((config) => config.is_default);
		if (!defaultModelConfig) {
			return "";
		}
		return modelIDByConfigID.get(defaultModelConfig.id) ?? "";
	}, [modelConfigs, modelIDByConfigID]);
	const preferredModelID =
		lastUsedModelID || defaultModelID || (modelOptions[0]?.id ?? "");
	const [userSelectedModel, setUserSelectedModel] = useState("");
	const [hasUserSelectedModel, setHasUserSelectedModel] = useState(false);
	// Derive the effective model every render so we never reference
	// a stale model id and can honor fallback precedence.
	const selectedModel =
		hasUserSelectedModel &&
		modelOptions.some((modelOption) => modelOption.id === userSelectedModel)
			? userSelectedModel
			: preferredModelID;
	const workspacesQuery = useQuery(workspaces({ limit: 50 }));
	const [selectedWorkspaceId, setSelectedWorkspaceId] = useState<string | null>(
		() => {
			if (typeof window === "undefined") return null;
			return localStorage.getItem(selectedWorkspaceIdStorageKey) || null;
		},
	);
	const workspaceOptions = workspacesQuery.data?.workspaces ?? [];
	const autoCreateWorkspaceValue = "__auto_create_workspace__";
	const hasModelOptions = modelOptions.length > 0;
	const hasConfiguredModels = hasConfiguredModelsInCatalog(modelCatalog);
	const modelSelectorPlaceholder = getModelSelectorPlaceholder(
		modelOptions,
		isModelCatalogLoading,
		hasConfiguredModels,
	);
	const modelCatalogStatusMessage = getModelCatalogStatusMessage(
		modelCatalog,
		modelOptions,
		isModelCatalogLoading,
		Boolean(modelCatalogError),
	);
	const inputStatusText = hasModelOptions
		? null
		: hasConfiguredModels
			? "Models are configured but unavailable. Ask an admin."
			: "No models configured. Ask an admin.";

	useEffect(() => {
		if (typeof window === "undefined") {
			return;
		}
		if (!initialLastModelConfigID) {
			return;
		}
		if (isModelCatalogLoading || isModelConfigsLoading) {
			return;
		}
		if (lastUsedModelID) {
			return;
		}
		localStorage.removeItem(lastModelConfigIDStorageKey);
	}, [
		initialLastModelConfigID,
		isModelCatalogLoading,
		isModelConfigsLoading,
		lastUsedModelID,
	]);

	// Keep a mutable ref to selectedWorkspaceId and selectedModel so
	// that the onSend callback always sees the latest values without
	// the shared input component re-rendering on every change.
	const selectedWorkspaceIdRef = useRef(selectedWorkspaceId);
	selectedWorkspaceIdRef.current = selectedWorkspaceId;
	const selectedModelRef = useRef(selectedModel);
	selectedModelRef.current = selectedModel;

	const handleWorkspaceChange = (value: string) => {
		if (value === autoCreateWorkspaceValue) {
			setSelectedWorkspaceId(null);
			if (typeof window !== "undefined") {
				localStorage.removeItem(selectedWorkspaceIdStorageKey);
			}
			return;
		}
		setSelectedWorkspaceId(value);
		if (typeof window !== "undefined") {
			localStorage.setItem(selectedWorkspaceIdStorageKey, value);
		}
	};

	const handleInputChange = useCallback((value: string) => {
		if (typeof window !== "undefined") {
			localStorage.setItem(emptyInputStorageKey, value);
		}
	}, []);
	const handleModelChange = useCallback((value: string) => {
		setHasUserSelectedModel(true);
		setUserSelectedModel(value);
	}, []);

	const handleSend = useCallback(
		async (message: string) => {
			await onCreateChat({
				message,
				workspaceId: selectedWorkspaceIdRef.current ?? undefined,
				model: selectedModelRef.current || undefined,
			});
		},
		[onCreateChat],
	);

	const selectedWorkspaceName = selectedWorkspaceId
		? workspaceOptions.find((ws) => ws.id === selectedWorkspaceId)?.name
		: null;

	return (
		<div className="flex h-full min-h-0 flex-1 items-center justify-center overflow-auto p-4 sm:p-6 lg:p-8">
			<div className="mx-auto flex w-full max-w-3xl flex-col gap-4">
				{createError ? <ErrorAlert error={createError} /> : null}
				{workspacesQuery.isError && (
					<ErrorAlert error={workspacesQuery.error} />
				)}

				<AgentChatInput
					onSend={handleSend}
					placeholder="Ask Coder to build, fix bugs, or explore your project..."
					isDisabled={isCreating}
					isLoading={isCreating}
					initialValue={initialInput}
					onInputChange={handleInputChange}
					selectedModel={selectedModel}
					onModelChange={handleModelChange}
					modelOptions={modelOptions}
					modelSelectorPlaceholder={modelSelectorPlaceholder}
					hasModelOptions={hasModelOptions}
					inputStatusText={inputStatusText}
					modelCatalogStatusMessage={modelCatalogStatusMessage}
					leftActions={
						<Select
							value={selectedWorkspaceId ?? autoCreateWorkspaceValue}
							onValueChange={handleWorkspaceChange}
							disabled={isCreating || workspacesQuery.isLoading}
						>
							<SelectTrigger className="h-8 w-auto gap-1.5 border-none bg-transparent px-1 text-xs shadow-none hover:bg-transparent">
								<MonitorIcon className="h-3.5 w-3.5 shrink-0 text-content-secondary" />
								<SelectValue>
									{selectedWorkspaceName ?? "Workspace"}
								</SelectValue>
							</SelectTrigger>
							<SelectContent
								side="top"
								align="center"
								className="[&_[role=option]]:text-xs"
							>
								<SelectItem value={autoCreateWorkspaceValue}>
									Auto-create Workspace
								</SelectItem>
								{workspaceOptions.map((workspace) => (
									<SelectItem key={workspace.id} value={workspace.id}>
										{workspace.name}
									</SelectItem>
								))}
								{workspaceOptions.length === 0 &&
									!workspacesQuery.isLoading && (
										<SelectItem value="no-workspaces" disabled>
											No workspaces found
										</SelectItem>
									)}
							</SelectContent>
						</Select>
					}
				/>
			</div>
		</div>
	);
};

export default AgentsPage;
