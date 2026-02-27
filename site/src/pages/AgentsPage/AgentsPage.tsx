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
import { ExternalImage } from "components/ExternalImage/ExternalImage";
import { CoderIcon } from "components/Icons/CoderIcon";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "components/Select/Select";
import { useAuthenticated } from "hooks";
import { ArrowLeftIcon, MonitorIcon, PanelLeftIcon } from "lucide-react";
import { UserDropdown } from "modules/dashboard/Navbar/UserDropdown/UserDropdown";
import { useDashboard } from "modules/dashboard/useDashboard";
import {
	type FC,
	type FormEvent,
	useCallback,
	useEffect,
	useMemo,
	useRef,
	useState,
} from "react";
import { createPortal } from "react-dom";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { NavLink, Outlet, useNavigate, useParams } from "react-router";
import { toast } from "sonner";
import { cn } from "utils/cn";
import { pageTitle } from "utils/page";
import { AgentChatInput } from "./AgentChatInput";
import { AgentsSidebar } from "./AgentsSidebar";
import { ConfigureAgentsDialog } from "./ConfigureAgentsDialog";
import { DiffRightPanel } from "./DiffRightPanel";
import {
	getModelCatalogStatusMessage,
	getModelOptionsFromCatalog,
	getModelSelectorPlaceholder,
	hasConfiguredModelsInCatalog,
} from "./modelOptions";

const emptyInputStorageKey = "agents.empty-input";
const selectedWorkspaceIdStorageKey = "agents.selected-workspace-id";
const lastModelConfigIDStorageKey = "agents.last-model-config-id";
const systemPromptStorageKey = "agents.system-prompt";
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
	topBarTitleRef: React.RefObject<HTMLDivElement | null>;
	topBarActionsRef: React.RefObject<HTMLDivElement | null>;
	rightPanelRef: React.RefObject<HTMLDivElement | null>;
	setRightPanelOpen: (isOpen: boolean) => void;
	requestArchiveAgent: (chatId: string) => void;
}

const AgentsPage: FC = () => {
	const queryClient = useQueryClient();
	const navigate = useNavigate();
	const { agentId } = useParams();
	const { permissions, user, signOut } = useAuthenticated();
	const { appearance, buildInfo } = useDashboard();
	const isAgentsAdmin =
		permissions.editDeploymentConfig ||
		user.roles.some((role) => role.name === "owner" || role.name === "admin");
	const canSetSystemPrompt = isAgentsAdmin;

	// The global CSS sets scrollbar-gutter: stable on <html> to prevent
	// layout shift on pages that toggle scrollbars. The agents page uses
	// its own internal scroll containers so the reserved gutter space is
	// unnecessary and wastes horizontal room.
	useEffect(() => {
		const html = document.documentElement;
		const prev = html.style.scrollbarGutter;
		html.style.scrollbarGutter = "auto";
		return () => {
			html.style.scrollbarGutter = prev;
		};
	}, []);

	const chatsQuery = useQuery(chats());
	const chatModelsQuery = useQuery(chatModels());
	const chatModelConfigsQuery = useQuery(chatModelConfigs());
	const createMutation = useMutation(createChat(queryClient));
	const archiveMutation = useMutation(deleteChat(queryClient));
	const [archivingChatId, setArchivingChatId] = useState<string | null>(null);
	const [isRightPanelOpen, setIsRightPanelOpen] = useState(false);
	const [isSidebarCollapsed, setIsSidebarCollapsed] = useState(false);
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
	const topBarTitleRef = useRef<HTMLDivElement>(null);
	const topBarActionsRef = useRef<HTMLDivElement>(null);
	const rightPanelRef = useRef<HTMLDivElement>(null);
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
			topBarTitleRef,
			topBarActionsRef,
			rightPanelRef,
			setRightPanelOpen: setIsRightPanelOpen,
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

	useEffect(() => {
		if (!agentId) {
			setIsRightPanelOpen(false);
		}
	}, [agentId]);

	return (
		<div className="flex h-full min-h-0 flex-col overflow-hidden bg-surface-primary md:flex-row">
			<div
				className={cn(
					"md:h-full md:w-[320px] md:min-h-0 md:border-b-0",
					agentId
						? "hidden md:block shrink-0 h-[42dvh] min-h-[240px] border-b border-border-default"
						: "order-2 md:order-none flex-1 min-h-0 border-t border-border-default md:flex-none md:border-t-0",
					isSidebarCollapsed && "md:hidden",
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
					onCollapse={() => setIsSidebarCollapsed(true)}
				/>
			</div>

			<div
				className={cn(
					"flex min-h-0 min-w-0 bg-surface-primary",
					agentId ? "flex-1" : "order-1 md:order-none flex-none md:flex-1",
					isRightPanelOpen && "flex-col xl:flex-row",
				)}
			>
				<div className="flex min-h-0 min-w-0 flex-1 flex-col bg-surface-primary">
					<div className="flex shrink-0 items-center gap-2 px-4 py-0.5">
						{/* Mobile logo: visible when no agent is selected. */}
						{!agentId && (
							<NavLink
								to="/workspaces"
								className="inline-flex shrink-0 opacity-50 md:hidden"
							>
								{appearance.logo_url ? (
									<ExternalImage
										className="h-6"
										src={appearance.logo_url}
										alt="Logo"
									/>
								) : (
									<CoderIcon className="h-6 w-6 fill-content-primary" />
								)}
							</NavLink>
						)}
						{/* Mobile back button: visible on mobile when an agent is selected. */}
						{agentId && (
							<Button
								variant="subtle"
								size="icon"
								onClick={() => navigate("/agents")}
								aria-label="Back"
								className="inline-flex h-7 w-7 min-w-0 shrink-0 md:hidden"
							>
								<ArrowLeftIcon />
							</Button>
						)}
						{/* Desktop expand button: visible when sidebar is manually collapsed. */}
						{isSidebarCollapsed && (
							<Button
								variant="subtle"
								size="icon"
								onClick={() => setIsSidebarCollapsed(false)}
								aria-label="Expand sidebar"
								className="hidden h-7 w-7 min-w-0 shrink-0 md:inline-flex"
							>
								<PanelLeftIcon />
							</Button>
						)}
						<div
							ref={topBarTitleRef}
							className="flex min-w-0 flex-1 items-center"
						/>
						<div ref={topBarActionsRef} className="flex items-center gap-2" />
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
					{agentId ? (
						<Outlet context={outletContext} />
					) : (
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
							canSetSystemPrompt={canSetSystemPrompt}
							canManageChatModelConfigs={isAgentsAdmin}
							topBarActionsRef={topBarActionsRef}
						/>
					)}
				</div>
				<DiffRightPanel
					ref={rightPanelRef}
					isOpen={Boolean(agentId && isRightPanelOpen)}
				/>
			</div>
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
	canSetSystemPrompt: boolean;
	canManageChatModelConfigs: boolean;
	topBarActionsRef: React.RefObject<HTMLDivElement | null>;
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
	canSetSystemPrompt,
	canManageChatModelConfigs,
	topBarActionsRef,
}) => {
	const initialInput = useMemo(() => {
		if (typeof window === "undefined") {
			return "";
		}
		return localStorage.getItem(emptyInputStorageKey) ?? "";
	}, []);
	const initialSystemPrompt = useMemo(() => {
		if (typeof window === "undefined") {
			return "";
		}
		return localStorage.getItem(systemPromptStorageKey) ?? "";
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
	const [savedSystemPrompt, setSavedSystemPrompt] =
		useState(initialSystemPrompt);
	const [systemPromptDraft, setSystemPromptDraft] =
		useState(initialSystemPrompt);
	const [isConfigureAgentsDialogOpen, setConfigureAgentsDialogOpen] =
		useState(false);
	const workspacesQuery = useQuery(workspaces({ limit: 50 }));
	const [selectedWorkspaceId, setSelectedWorkspaceId] = useState<string | null>(
		() => {
			if (typeof window === "undefined") return null;
			return localStorage.getItem(selectedWorkspaceIdStorageKey) || null;
		},
	);
	const workspaceOptions = workspacesQuery.data?.workspaces ?? [];
	const autoCreateWorkspaceValue = "__auto_create_workspace__";
	const hasAdminControls = canSetSystemPrompt || canManageChatModelConfigs;
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
	const isSystemPromptDirty = systemPromptDraft !== savedSystemPrompt;

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

	const handleSaveSystemPrompt = useCallback(
		(event: FormEvent) => {
			event.preventDefault();
			if (!isSystemPromptDirty) {
				return;
			}

			setSavedSystemPrompt(systemPromptDraft);
			if (typeof window !== "undefined") {
				if (systemPromptDraft) {
					localStorage.setItem(systemPromptStorageKey, systemPromptDraft);
				} else {
					localStorage.removeItem(systemPromptStorageKey);
				}
			}
		},
		[isSystemPromptDirty, systemPromptDraft],
	);

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
		<div className="flex min-h-0 flex-1 items-start justify-center overflow-auto p-4 pt-12 md:h-full md:items-center md:pt-4">
			{hasAdminControls &&
				topBarActionsRef.current &&
				createPortal(
					<Button
						variant="subtle"
						disabled={isCreating}
						className="h-8 gap-1.5 border-none bg-transparent px-1 text-[13px] shadow-none hover:bg-transparent"
						onClick={() => setConfigureAgentsDialogOpen(true)}
					>
						Admin
					</Button>,
					topBarActionsRef.current,
				)}

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
							<SelectTrigger className="h-8 w-auto gap-1.5 border-none bg-transparent px-1 text-xs shadow-none transition-colors hover:bg-transparent hover:text-content-primary [&>svg]:transition-colors [&>svg]:hover:text-content-primary focus:ring-0 focus-visible:ring-0">
								<MonitorIcon className="h-3.5 w-3.5 shrink-0 text-content-secondary group-hover:text-content-primary" />
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

			{hasAdminControls && (
				<ConfigureAgentsDialog
					open={isConfigureAgentsDialogOpen}
					onOpenChange={setConfigureAgentsDialogOpen}
					canManageChatModelConfigs={canManageChatModelConfigs}
					canSetSystemPrompt={canSetSystemPrompt}
					systemPromptDraft={systemPromptDraft}
					onSystemPromptDraftChange={setSystemPromptDraft}
					onSaveSystemPrompt={handleSaveSystemPrompt}
					isSystemPromptDirty={isSystemPromptDirty}
					isDisabled={isCreating}
				/>
			)}
		</div>
	);
};

export default AgentsPage;
