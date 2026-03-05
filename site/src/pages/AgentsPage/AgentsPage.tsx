import { API, watchChats } from "api/api";
import { getErrorMessage } from "api/errors";
import {
	archiveChat,
	chatDiffContentsKey,
	chatDiffStatusKey,
	chatKey,
	chatModelConfigs,
	chatModels,
	chats,
	chatsKey,
	createChat,
	unarchiveChat,
} from "api/queries/chats";
import { workspaces } from "api/queries/workspaces";
import type * as TypesGen from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { ChevronDownIcon } from "components/AnimatedIcons/ChevronDown";
import type { ModelSelectorOption } from "components/ai-elements";
import { Button } from "components/Button/Button";
import {
	Combobox,
	ComboboxContent,
	ComboboxEmpty,
	ComboboxInput,
	ComboboxItem,
	ComboboxList,
	ComboboxTrigger,
} from "components/Combobox/Combobox";
import { ExternalImage } from "components/ExternalImage/ExternalImage";
import { CoderIcon } from "components/Icons/CoderIcon";
import { useAuthenticated } from "hooks";
import { MonitorIcon, PanelLeftIcon } from "lucide-react";
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
import { useMutation, useQuery, useQueryClient } from "react-query";
import { NavLink, Outlet, useNavigate, useParams } from "react-router";
import { toast } from "sonner";
import { cn } from "utils/cn";
import { pageTitle } from "utils/page";
import { AgentChatInput } from "./AgentChatInput";
import { maybePlayChime } from "./AgentDetail/useAgentChime";
import { AgentsSidebar } from "./AgentsSidebar";
import { ChimeButton } from "./ChimeButton";
import { ConfigureAgentsDialog } from "./ConfigureAgentsDialog";
import {
	getModelCatalogStatusMessage,
	getModelOptionsFromCatalog,
	getModelSelectorPlaceholder,
	hasConfiguredModelsInCatalog,
} from "./modelOptions";
import { useAgentsPageKeybindings } from "./useAgentsPageKeybindings";
import { useAgentsPWA } from "./useAgentsPWA";
import { WebPushButton } from "./WebPushButton";

/** @internal Exported for testing. */
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
	requestArchiveAgent: (chatId: string) => void;
	requestUnarchiveAgent: (chatId: string) => void;
	requestArchiveAndDeleteWorkspace: (
		chatId: string,
		workspaceId: string,
	) => void;
	isSidebarCollapsed: boolean;
	onToggleSidebarCollapsed: () => void;
}

const AgentsPage: FC = () => {
	useAgentsPWA();
	const queryClient = useQueryClient();
	const navigate = useNavigate();
	const { agentId } = useParams();
	const { permissions, user } = useAuthenticated();
	const { appearance } = useDashboard();
	const isAgentsAdmin =
		permissions.editDeploymentConfig ||
		user.roles.some((role) => role.name === "owner" || role.name === "admin");
	const canSetSystemPrompt = isAgentsAdmin;

	// The global CSS sets scrollbar-gutter: stable on <html> to prevent
	// layout shift on pages that toggle scrollbars. The agents page
	// uses its own internal scroll containers so the reserved gutter
	// space is unnecessary and wastes horizontal room.
	//
	// Removing the gutter requires three things:
	//
	// 1. overflow:hidden on both <html> and <body> so neither element
	//    can produce a scrollbar.
	// 2. scrollbar-gutter:auto on <html> so the browser stops
	//    reserving space for a scrollbar that will never appear.
	//    This is what makes react-remove-scroll-bar measure a gap of
	//    0 when a Radix dropdown opens, so it injects no padding or
	//    margin compensation.
	// 3. An injected <style> that overrides the global
	//    `overflow-y: scroll !important` on body[data-scroll-locked].
	//    Without this, opening any Radix dropdown would force a
	//    scrollbar onto <body>, re-introducing the layout shift.
	useEffect(() => {
		const html = document.documentElement;
		const body = document.body;

		const prevHtmlOverflow = html.style.overflow;
		const prevHtmlScrollbarGutter = html.style.scrollbarGutter;
		const prevBodyOverflow = body.style.overflow;

		html.style.overflow = "hidden";
		html.style.scrollbarGutter = "auto";
		body.style.overflow = "hidden";

		const style = document.createElement("style");
		style.textContent =
			"html body[data-scroll-locked] { overflow-y: hidden !important; }";
		document.head.appendChild(style);

		return () => {
			html.style.overflow = prevHtmlOverflow;
			html.style.scrollbarGutter = prevHtmlScrollbarGutter;
			body.style.overflow = prevBodyOverflow;
			style.remove();
		};
	}, []);

	const chatsQuery = useQuery(chats());
	const chatModelsQuery = useQuery(chatModels());
	const chatModelConfigsQuery = useQuery(chatModelConfigs());
	const createMutation = useMutation(createChat(queryClient));
	const archiveChatBase = archiveChat(queryClient);
	const archiveAgentMutation = useMutation({
		...archiveChatBase,
		onSuccess: (_data, chatId) => {
			clearChatErrorReason(chatId);
			toast.success("Agent archived.");
		},
		onError: (error, chatId, context) => {
			archiveChatBase.onError(error, chatId, context);
			toast.error(getErrorMessage(error, "Failed to archive agent."));
		},
	});
	const archiveAndDeleteMutation = useMutation({
		mutationFn: async ({
			chatId,
			workspaceId,
		}: {
			chatId: string;
			workspaceId: string;
		}) => {
			await API.archiveChat(chatId);
			await API.deleteWorkspace(workspaceId);
			return { chatId, workspaceId };
		},
		onSuccess: async ({ chatId }) => {
			clearChatErrorReason(chatId);
			await queryClient.invalidateQueries({ queryKey: chatsKey });
			await queryClient.invalidateQueries({ queryKey: chatKey(chatId) });
			toast.success("Agent archived.");
			toast.success("Workspace deletion initiated.");
		},
		onError: (error) => {
			toast.error(getErrorMessage(error, "Failed to archive agent."));
		},
	});
	const unarchiveChatBase = unarchiveChat(queryClient);
	const unarchiveAgentMutation = useMutation({
		...unarchiveChatBase,
		onSuccess: () => {
			toast.success("Agent unarchived.");
		},
		onError: (error, chatId, context) => {
			unarchiveChatBase.onError(error, chatId, context);
			toast.error(getErrorMessage(error, "Failed to unarchive agent."));
		},
	});
	const [isConfigureAgentsDialogOpen, setConfigureAgentsDialogOpen] =
		useState(false);
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
	const chatList = chatsQuery.data ?? [];
	const isArchiving =
		archiveAgentMutation.isPending || archiveAndDeleteMutation.isPending;
	const archivingChatId =
		(archiveAgentMutation.isPending
			? archiveAgentMutation.variables
			: undefined) ??
		(archiveAndDeleteMutation.isPending
			? archiveAndDeleteMutation.variables?.chatId
			: undefined);
	const requestArchiveAgent = useCallback(
		(chatId: string) => {
			if (!isArchiving) {
				archiveAgentMutation.mutate(chatId);
			}
		},
		[isArchiving, archiveAgentMutation],
	);
	const requestArchiveAndDeleteWorkspace = useCallback(
		(chatId: string, workspaceId: string) => {
			if (!isArchiving) {
				archiveAndDeleteMutation.mutate({ chatId, workspaceId });
			}
		},
		[isArchiving, archiveAndDeleteMutation],
	);
	const requestUnarchiveAgent = useCallback(
		(chatId: string) => {
			unarchiveAgentMutation.mutate(chatId);
		},
		[unarchiveAgentMutation],
	);
	const handleToggleSidebarCollapsed = useCallback(
		() => setIsSidebarCollapsed((prev) => !prev),
		[],
	);
	const outletContext: AgentsOutletContext = useMemo(
		() => ({
			chatErrorReasons,
			setChatErrorReason,
			clearChatErrorReason,
			requestArchiveAgent,
			requestUnarchiveAgent,
			requestArchiveAndDeleteWorkspace,
			isSidebarCollapsed,
			onToggleSidebarCollapsed: handleToggleSidebarCollapsed,
		}),
		[
			chatErrorReasons,
			setChatErrorReason,
			clearChatErrorReason,
			requestArchiveAgent,
			requestUnarchiveAgent,
			requestArchiveAndDeleteWorkspace,
			isSidebarCollapsed,
			handleToggleSidebarCollapsed,
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
		// Only clear the draft when the user is already on the empty
		// state and explicitly requests a blank slate.  When navigating
		// back from a conversation the existing draft is preserved.
		if (typeof window !== "undefined" && !agentId) {
			localStorage.removeItem(emptyInputStorageKey);
		}
		navigate("/agents");
	};

	// Track the active chat ID in a ref so the watchChats
	// WebSocket handler can read it without re-subscribing on
	// every navigation.
	const activeChatIDRef = useRef(agentId);
	activeChatIDRef.current = agentId;

	useEffect(() => {
		const ws = watchChats();
		ws.addEventListener("open", () => {
			void queryClient.invalidateQueries({ queryKey: chatsKey });
		});
		ws.addEventListener("close", () => {
			void queryClient.invalidateQueries({ queryKey: chatsKey });
		});
		ws.addEventListener("error", () => {
			void queryClient.invalidateQueries({ queryKey: chatsKey });
		});
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

			// Read the previous status from the query cache, which
			// is synchronously updated by both the per-chat WebSocket
			// (via updateSidebarChat) and this handler. This avoids
			// the async-lag of a useEffect-based status map.
			const currentChats = queryClient.getQueryData<TypesGen.Chat[]>(chatsKey);
			const prevStatus = currentChats?.find(
				(c) => c.id === updatedChat.id,
			)?.status;
			// Only play the chime for top-level chats, not sub-agents.
			if (!updatedChat.parent_chat_id) {
				maybePlayChime(
					prevStatus,
					updatedChat.status,
					updatedChat.id,
					activeChatIDRef.current,
				);
			}

			if (chatEvent.kind === "deleted") {
				queryClient.setQueryData(
					chatsKey,
					(prev: TypesGen.Chat[] | undefined) =>
						prev?.filter(
							(c) =>
								c.id !== updatedChat.id && c.root_chat_id !== updatedChat.id,
						),
				);
				queryClient.removeQueries({
					queryKey: chatKey(updatedChat.id),
					exact: true,
				});
				return;
			}

			if (chatEvent.kind === "diff_status_change") {
				void Promise.all([
					queryClient.invalidateQueries({
						queryKey: chatsKey,
					}),
					queryClient.invalidateQueries({
						queryKey: chatDiffStatusKey(updatedChat.id),
					}),
					queryClient.invalidateQueries({
						queryKey: chatDiffContentsKey(updatedChat.id),
					}),
				]);
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
										updated_at:
											c.updated_at > updatedChat.updated_at
												? c.updated_at
												: updatedChat.updated_at,
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
							updated_at:
								previousChat.chat.updated_at > updatedChat.updated_at
									? previousChat.chat.updated_at
									: updatedChat.updated_at,
						},
					};
				},
			);
		});
		return () => {
			ws.close();
		};
	}, [queryClient]);

	useEffect(() => {
		document.title = pageTitle("Agents");
	}, []);

	useAgentsPageKeybindings({
		onNewAgent: handleNewAgent,
	});

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
					onUnarchiveAgent={requestUnarchiveAgent}
					onArchiveAndDeleteWorkspace={requestArchiveAndDeleteWorkspace}
					onNewAgent={handleNewAgent}
					isCreating={createMutation.isPending}
					isArchiving={isArchiving}
					archivingChatId={archivingChatId}
					isLoading={chatsQuery.isLoading}
					loadError={chatsQuery.isError ? chatsQuery.error : undefined}
					onRetryLoad={() => void chatsQuery.refetch()}
					onCollapse={() => setIsSidebarCollapsed(true)}
				/>
			</div>

			<div
				className={cn(
					"flex min-h-0 min-w-0 flex-1 flex-col bg-surface-primary",
					!agentId && "order-1 md:order-none flex-none md:flex-1",
				)}
			>
				{agentId ? (
					<Outlet key={agentId} context={outletContext} />
				) : (
					<>
						<div className="flex shrink-0 items-center gap-2 px-4 py-0.5">
							<NavLink
								to="/workspaces"
								className="inline-flex shrink-0 md:hidden"
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
							<div className="flex min-w-0 flex-1 items-center" />
							<div className="flex items-center gap-2">
								<ChimeButton />
								<WebPushButton />{" "}
								{isAgentsAdmin && (
									<Button
										variant="subtle"
										disabled={createMutation.isPending}
										className="h-8 gap-1.5 border-none bg-transparent px-1 text-[13px] shadow-none hover:bg-transparent"
										onClick={() => setConfigureAgentsDialogOpen(true)}
									>
										Admin
									</Button>
								)}
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
							canSetSystemPrompt={canSetSystemPrompt}
							canManageChatModelConfigs={isAgentsAdmin}
							isConfigureAgentsDialogOpen={isConfigureAgentsDialogOpen}
							onConfigureAgentsDialogOpenChange={setConfigureAgentsDialogOpen}
						/>
					</>
				)}
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
	isConfigureAgentsDialogOpen: boolean;
	onConfigureAgentsDialogOpenChange: (open: boolean) => void;
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
	isConfigureAgentsDialogOpen,
	onConfigureAgentsDialogOpenChange,
}) => {
	const [initialInputValue] = useState(() => {
		if (typeof window === "undefined") {
			return "";
		}
		return localStorage.getItem(emptyInputStorageKey) ?? "";
	});
	const inputValueRef = useRef(initialInputValue);
	const initialSystemPrompt = () => {
		if (typeof window === "undefined") {
			return "";
		}
		return localStorage.getItem(systemPromptStorageKey) ?? "";
	};
	const [initialLastModelConfigID] = useState(() => {
		if (typeof window === "undefined") {
			return "";
		}
		return localStorage.getItem(lastModelConfigIDStorageKey) ?? "";
	});
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
	const workspacesQuery = useQuery(workspaces({ q: "owner:me", limit: 0 }));
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

	const handleContentChange = useCallback((content: string) => {
		inputValueRef.current = content;
		if (typeof window !== "undefined") {
			if (content) {
				localStorage.setItem(emptyInputStorageKey, content);
			} else {
				localStorage.removeItem(emptyInputStorageKey);
			}
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
		(message: string) => {
			// Clear the draft synchronously before the async
			// onCreateChat call so that editor change events
			// firing during the async gap cannot re-persist it.
			localStorage.removeItem(emptyInputStorageKey);
			void onCreateChat({
				message,
				workspaceId: selectedWorkspaceIdRef.current ?? undefined,
				model: selectedModelRef.current || undefined,
			});
		},
		[onCreateChat],
	);

	const selectedWorkspace = selectedWorkspaceId
		? workspaceOptions.find((ws) => ws.id === selectedWorkspaceId)
		: undefined;
	const selectedWorkspaceLabel = selectedWorkspace
		? `${selectedWorkspace.owner_name}/${selectedWorkspace.name}`
		: undefined;

	return (
		<div className="flex min-h-0 flex-1 items-start justify-center overflow-auto p-4 pt-12 md:h-full md:items-center md:pt-4">
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
					initialValue={initialInputValue}
					onContentChange={handleContentChange}
					selectedModel={selectedModel}
					onModelChange={handleModelChange}
					modelOptions={modelOptions}
					modelSelectorPlaceholder={modelSelectorPlaceholder}
					hasModelOptions={hasModelOptions}
					inputStatusText={inputStatusText}
					modelCatalogStatusMessage={modelCatalogStatusMessage}
					leftActions={
						<Combobox
							value={selectedWorkspaceId ?? autoCreateWorkspaceValue}
							onValueChange={(value) =>
								handleWorkspaceChange(value ?? autoCreateWorkspaceValue)
							}
						>
							<ComboboxTrigger asChild>
								<button
									type="button"
									disabled={isCreating || workspacesQuery.isLoading}
									className="group flex h-8 items-center gap-1.5 border-none bg-transparent px-1 text-xs text-content-secondary shadow-none transition-colors hover:bg-transparent hover:text-content-primary cursor-pointer disabled:cursor-not-allowed disabled:opacity-50"
								>
									<MonitorIcon className="h-3.5 w-3.5 shrink-0 text-content-secondary" />
									<span>{selectedWorkspaceLabel ?? "Workspace"}</span>
									<ChevronDownIcon className="size-icon-sm text-content-secondary transition-colors group-hover:text-content-primary" />
								</button>
							</ComboboxTrigger>
							<ComboboxContent
								side="top"
								align="center"
								className="w-72 [&_[cmdk-item]]:text-xs"
							>
								<ComboboxInput placeholder="Search workspaces..." />
								<ComboboxList>
									<ComboboxItem value={autoCreateWorkspaceValue}>
										Auto-create Workspace
									</ComboboxItem>
									{workspaceOptions.map((workspace) => (
										<ComboboxItem
											key={workspace.id}
											value={workspace.id}
											keywords={[workspace.owner_name, workspace.name]}
										>
											{workspace.owner_name}/{workspace.name}
										</ComboboxItem>
									))}
								</ComboboxList>
								<ComboboxEmpty>No workspaces found</ComboboxEmpty>
							</ComboboxContent>
						</Combobox>
					}
				/>
			</div>

			{hasAdminControls && (
				<ConfigureAgentsDialog
					open={isConfigureAgentsDialogOpen}
					onOpenChange={onConfigureAgentsDialogOpenChange}
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
