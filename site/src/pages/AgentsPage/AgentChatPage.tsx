import {
	MessageScroller,
	useMessageScroller,
} from "@shadcn/react/message-scroller";
import { type FC, useEffect, useRef, useState } from "react";

import {
	useInfiniteQuery,
	useMutation,
	useQuery,
	useQueryClient,
} from "react-query";
import { useOutletContext, useParams } from "react-router";
import { toast } from "sonner";
import type { UrlTransform } from "streamdown";
import { getErrorMessage, getErrorStatus, isApiError } from "#/api/errors";
import { chatProviderConfigs } from "#/api/queries/aiProviders";
import {
	chatMessagesForInfiniteScroll,
	chatModels,
	chatQueueConvergence,
	clearChat,
	compactChat,
	createChatMessage,
	deleteChatQueuedMessage,
	editChatMessage,
	getOpenChatPollInterval,
	interruptChat,
	invalidateChatEntity,
	mcpServerConfigs,
	openChat,
	patchChatEntity,
	promoteChatQueuedMessage,
	updateChatPlanMode,
	updateChatWorkspace,
	updateInfiniteChatsCache,
	userChatDebugLogging,
} from "#/api/queries/chats";
import { deploymentSSHConfig } from "#/api/queries/deployment";
import { userSkills } from "#/api/queries/userSkills";
import { workspaceById, workspaceByIdKey } from "#/api/queries/workspaces";
import type * as TypesGen from "#/api/typesGenerated";
import { useProxy } from "#/contexts/ProxyContext";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useAIGatewayEnabled } from "#/hooks/useEmbeddedMetadata";
import {
	getDefaultOrganizationName,
	useDashboard,
} from "#/modules/dashboard/useDashboard";
import { pageTitle } from "#/utils/page";
import { rewriteLocalhostURL } from "#/utils/portForward";
import { AgentChatPageErrorView } from "./AgentChatPageErrorView";
import {
	AgentChatPageLoadingView,
	AgentChatPageNotFoundView,
	AgentChatPageView,
} from "./AgentChatPageView";
import type { AgentsPageOutletContext } from "./AgentsPageLayout";
import type { ChatMessageInputRef } from "./components/AgentChatInput";
import {
	type ChatDetailError,
	getPersistedDetailError,
	isChatHookDeniedResponse,
	isChatHookDispatchFailedResponse,
} from "./components/ChatConversation/chatError";
import { getWorkspaceAgent } from "./components/ChatConversation/chatHelpers";
import { runPromoteQueuedMessage } from "./components/ChatConversation/chatQueueReconciliation";
import {
	selectChatStatus,
	useChatSelector,
	useChatStore,
} from "./components/ChatConversation/chatStore";
import { submitChatTurn } from "./components/ChatConversation/submitChatTurn";
import { useChatToolInvalidations } from "./components/ChatConversation/useChatToolInvalidations";
import { useWorkspaceWatch } from "./components/ChatConversation/useWorkspaceWatch";
import { isChatAgentBindingUnresolved } from "./components/ChatConversation/watchedWorkspace";
import type { PendingAttachment } from "./components/ChatPageContent";
import { workspaceSkillsFromChat } from "./components/ChatPageContent";
import {
	getDefaultMCPSelection,
	getSavedMCPSelection,
	saveMCPSelection,
} from "./components/MCPServerPicker";
import { getModelSelectorHelp } from "./components/ModelSelectorHelp";
import { useAgentChatPanelPreference } from "./components/RightPanel/useAgentChatPanelPreference";
import { useConversationEditingState } from "./hooks/useConversationEditingState";
import { useGitWatcher } from "./hooks/useGitWatcher";
import {
	draftInputStorageKeyPrefix,
	parseStoredDraft,
} from "./utils/draftStorage";
import {
	countConfiguredProviderConfigs,
	getModelSelectorPlaceholder,
	getUnsupportedProviderNames,
	getUsableDefaultModelIDForOrganization,
	hasUserFixableProviders,
	isUnavailableHistoricalModelID,
	resolveModelOptionId,
	resolveModelSelector,
} from "./utils/modelOptions";
import { pickReasoningEffort } from "./utils/reasoningEffort";

const AGENT_BINDING_REPAIR_POLL_MS = 30_000;

const AgentChatPage: FC = () => {
	const { agentId } = useParams() as { agentId: string };
	const {
		chatErrorReasons,
		setChatErrorReason,
		clearChatErrorReason,
		onChatReady,
	} = useOutletContext<AgentsPageOutletContext>();
	const queryClient = useQueryClient();
	const { permissions, user: currentUser } = useAuthenticated();
	const { organizations, experiments } = useDashboard();
	const organizationName = getDefaultOrganizationName(organizations);
	const [selectedModel, setSelectedModel] = useState("");
	const [selectedReasoningEffort, setSelectedReasoningEffort] = useState("");
	const isEditReasoningEffortDirtyRef = useRef(false);
	const chatInputRef = useRef<ChatMessageInputRef | null>(null);
	const inputValueRef = useRef(
		parseStoredDraft(
			localStorage.getItem(`${draftInputStorageKeyPrefix}${agentId}`),
		).text,
	);

	const { showSidebarPanel, handleSetShowSidebarPanel } =
		useAgentChatPanelPreference();

	const chatQuery = useQuery({
		...openChat(agentId),
		// Poll while the chat runs (this override replaces openChat's
		// interval, and queued_for_capacity depends on the poll) or while
		// the binding is unresolved: repair happens on chat reads and watch
		// events cannot be relied on for retries because an idle workspace
		// publishes none.
		refetchInterval: ({ state }) => {
			const openPollMs = getOpenChatPollInterval(state.data);
			if (openPollMs !== false) {
				return openPollMs;
			}
			const workspaceId = state.data?.workspace_id;
			const workspace = workspaceId
				? queryClient.getQueryData<TypesGen.Workspace>(
						workspaceByIdKey(workspaceId),
					)
				: undefined;
			return isChatAgentBindingUnresolved(workspace, state.data?.agent_id)
				? AGENT_BINDING_REPAIR_POLL_MS
				: false;
		},
		refetchIntervalInBackground: false,
	});
	const chatOrganizationId = chatQuery.data?.organization_id ?? "";
	const chatMessagesQuery = useInfiniteQuery(
		chatMessagesForInfiniteScroll(agentId),
	);
	const workspaceId = chatQuery.data?.workspace_id;
	const chatAgentId = chatQuery.data?.agent_id;
	const workspaceQuery = useQuery({
		...workspaceById(workspaceId ?? ""),
		enabled: Boolean(workspaceId),
	});
	const workspace = workspaceQuery.data;

	const modelsQuery = useQuery(chatModels(chatOrganizationId));
	const models = modelsQuery.data?.models ?? [];
	const chatProviderConfigsQuery = useQuery({
		...chatProviderConfigs(),
		enabled: permissions.editDeploymentConfig,
	});
	const userDebugLoggingQuery = useQuery(userChatDebugLogging());
	const mcpServersQuery = useQuery({
		...mcpServerConfigs(chatOrganizationId),
		enabled: Boolean(chatOrganizationId),
	});
	const isDefaultChatOrganization = organizations.some(
		(organization) =>
			organization.id === chatOrganizationId && organization.is_default,
	);
	const desktopEnabled = experiments.includes("chat-virtual-desktop");
	const debugLoggingEnabled = Boolean(
		userDebugLoggingQuery.data?.debug_logging_enabled,
	);

	// MCP server selection state.
	const mcpServers = mcpServersQuery.data ?? [];
	const [selectedMCPServerIds, setSelectedMCPServerIds] = useState<
		string[] | null
	>(null);

	const handleMCPSelectionChange = (ids: string[]) => {
		setSelectedMCPServerIds(ids);
		if (chatOrganizationId) {
			saveMCPSelection(chatOrganizationId, ids);
		}
	};

	const handleMCPAuthComplete = (_serverId: string) => {
		void mcpServersQuery.refetch();
	};

	const {
		options: modelOptions,
		isModelCatalogLoading,
		modelCatalog,
		hasConfiguredModels,
	} = resolveModelSelector(chatOrganizationId, modelsQuery);
	const isModelDataPending = chatOrganizationId === "" || isModelCatalogLoading;
	const providerCount =
		permissions.editDeploymentConfig &&
		chatProviderConfigsQuery.data &&
		modelsQuery.data
			? countConfiguredProviderConfigs(
					chatProviderConfigsQuery.data,
					modelsQuery.data,
				)
			: undefined;
	const modelCount = modelsQuery.data ? modelOptions.length : undefined;
	const unsupportedProviderNames = getUnsupportedProviderNames(
		modelsQuery.data,
	);

	useWorkspaceWatch({
		workspaceId,
		agentId,
		chatAgentId,
	});
	const sshConfigQuery = useQuery(deploymentSSHConfig());
	const workspaceAgent = getWorkspaceAgent(workspace, chatAgentId);
	const { proxy } = useProxy();

	const chat = chatQuery.data;
	const isArchived = Boolean(chat?.archived);
	const isViewerNotOwner =
		chat !== undefined && currentUser.id !== chat.owner_id;
	const planModeEnabled = chat?.plan_mode === "plan";

	// Initialize MCP selection from chat record or defaults.
	const effectiveMCPServerIds = (() => {
		if (selectedMCPServerIds !== null) {
			return selectedMCPServerIds;
		}
		// If the chat has MCP server IDs recorded (even empty, meaning
		// the user deliberately opted out), use those.
		if (chat?.mcp_server_ids) {
			return chat.mcp_server_ids;
		}
		const saved = chatOrganizationId
			? getSavedMCPSelection(
					chatOrganizationId,
					mcpServers,
					isDefaultChatOrganization,
				)
			: null;
		if (saved !== null) {
			return saved;
		}
		// Otherwise, compute defaults from server availability.
		return getDefaultMCPSelection(mcpServers);
	})();

	// Flatten paginated messages into chronological order.
	// Pages arrive newest-first per page, and pages[0] is the
	// most recent page.
	const chatMessagesList = (() => {
		const pages = chatMessagesQuery.data?.pages;
		if (!pages || pages.length === 0) return undefined;
		// Collect all messages and deduplicate by ID as a defense
		// against cross-page duplicates. Cache upserts fan the same
		// fresh value out to every page containing an ID, so any
		// surviving duplicates are value-identical and either
		// occurrence is safe to render.
		const all = pages.flatMap((p) => p.messages);
		const byID = new Map(all.map((m) => [m.id, m]));
		const deduped = Array.from(byID.values());
		// Sort ascending by ID for chronological order.
		deduped.sort((a, b) => a.id - b.id);
		return deduped;
	})();

	// Queued messages are only in the first page (most recent).
	const chatQueuedMessages = chatMessagesQuery.data?.pages[0]?.queued_messages;

	// Build a synthetic ChatMessagesResponse from the flattened
	// data for backward compat with useChatStore.
	const chatMessagesData: TypesGen.ChatMessagesResponse | undefined =
		chatMessagesList
			? {
					messages: chatMessagesList,
					queued_messages: chatQueuedMessages ?? [],
					has_more: Boolean(chatMessagesQuery.data?.pages.at(-1)?.has_more),
				}
			: undefined;
	const chatLastModelConfigID = chat?.last_model_config_id;

	// Destructure mutation results directly so the React Compiler
	// tracks stable primitives/functions instead of the whole result
	// object (TanStack Query v5 recreates it every render via object
	// spread). Keeping no intermediate variable prevents future code
	// from accidentally closing over the unstable object.
	const { isPending: isSendPending, mutateAsync: sendMessage } = useMutation(
		createChatMessage(queryClient, agentId),
	);
	const { isPending: isEditPending, mutateAsync: editMessage } = useMutation(
		editChatMessage(queryClient, agentId),
	);
	const { isPending: isInterruptPending, mutateAsync: interrupt } = useMutation(
		interruptChat(queryClient, agentId),
	);
	const { isPending: isCompactPending, mutateAsync: compact } = useMutation(
		compactChat(queryClient, agentId),
	);
	const { isPending: isClearPending, mutateAsync: clearChatContext } =
		useMutation(clearChat(queryClient, agentId));
	const personalSkillsQuery = useQuery({
		...userSkills(),
		staleTime: 60_000,
	});
	const chatWorkspaceSkills = workspaceSkillsFromChat(chatQuery.data);
	const { mutateAsync: deleteQueuedMessage } = useMutation(
		deleteChatQueuedMessage(queryClient, agentId),
	);
	const { mutateAsync: promoteQueuedMessage } = useMutation(
		promoteChatQueuedMessage(queryClient, agentId),
	);
	const updateChatWorkspaceBase = updateChatWorkspace(queryClient);
	const {
		isPending: isUpdateChatWorkspacePending,
		mutateAsync: updateChatWorkspaceAsync,
	} = useMutation({
		...updateChatWorkspaceBase,
		onError: (error, variables, context) => {
			updateChatWorkspaceBase.onError(error, variables, context);
			toast.error(getErrorMessage(error, "Failed to update workspace."));
		},
	});

	const updateChatPlanModeBase = updateChatPlanMode(queryClient);
	const {
		isPending: isUpdateChatPlanModePending,
		mutateAsync: updateChatPlanModeAsync,
	} = useMutation({
		...updateChatPlanModeBase,
		onError: (error, variables, context) => {
			updateChatPlanModeBase.onError(error, variables, context);
			toast.error(getErrorMessage(error, "Failed to update plan mode."));
		},
	});
	const setCachedChatPlanMode = (
		chatId: string,
		planMode?: TypesGen.ChatPlanMode,
	) => {
		updateInfiniteChatsCache(queryClient, (chats) =>
			chats.map((chat) =>
				chat.id === chatId ? { ...chat, plan_mode: planMode } : chat,
			),
		);
		patchChatEntity(queryClient, chatId, (previousChat) =>
			previousChat ? { ...previousChat, plan_mode: planMode } : previousChat,
		);
	};

	const pendingPlanModeSyncRef = useRef<Promise<unknown> | null>(null);
	const pendingWorkspaceSyncRef = useRef<Promise<unknown> | null>(null);
	const trackPendingChatSettingSync = (
		syncPromise: Promise<unknown>,
		syncRef: { current: Promise<unknown> | null },
	) => {
		const trackedSync: Promise<unknown> = syncPromise.finally(() => {
			if (syncRef.current === trackedSync) {
				syncRef.current = null;
			}
		});
		syncRef.current = trackedSync;
		void trackedSync.catch(() => undefined);
	};

	const aiGatewayDisabled = !useAIGatewayEnabled();
	const { scrollToEnd } = useMessageScroller();
	const {
		store,
		isHydratingMessages,
		acceptServerChatStatus,
		setCacheQueuedMessages,
		getCacheQueuedMessages,
		upsertCacheMessages,
	} = useChatStore({
		chatID: agentId,
		chatMessages: chatMessagesList,
		chatRecord: chat,
		chatRecordUpdatedAt: chatQuery.dataUpdatedAt,
		chatMessagesData,
		chatQueuedMessages,
		setChatErrorReason,
		clearChatErrorReason,
		aiGatewayDisabled,
	});
	const liveChatStatus =
		useChatSelector(store, selectChatStatus) ?? chat?.status ?? null;
	const persistedError = getPersistedDetailError({
		chatStatus: liveChatStatus,
		chatRecord: chat,
		cachedError: chatErrorReasons[agentId],
	});

	// Git watcher: runs regardless of sidebar visibility, but only
	// connects when the workspace agent is in the "connected" state
	// to avoid an infinite reconnect loop against a missing agent.
	const gitWatcher = useGitWatcher({
		chatId: agentId,
		agentStatus: workspaceAgent?.status,
	});

	// Detect completed chat tool results so sidebar data stays in sync
	// with the server state those tools may have changed.
	useChatToolInvalidations({
		store,
		chatID: agentId,
		organizationName,
		username: currentUser.username,
	});

	const handleCommit = (repoRoot: string) => {
		const commitPrompt = `Commit and push the working changes in ${repoRoot}. If there are unstaged files, commit them too.`;
		const current = inputValueRef.current;
		if (current.includes(commitPrompt)) {
			return;
		}
		const prefix = current.trim() ? "\n\n" : "";
		chatInputRef.current?.insertText(prefix + commitPrompt);
		chatInputRef.current?.focus();
	};

	// Validate explicit and historical choices against organization options.
	// Prefer the usable organization default before another organization model.
	const effectiveSelectedModel = (() => {
		const resolvedSelectedModel = resolveModelOptionId(
			selectedModel,
			modelOptions,
		);
		if (resolvedSelectedModel) {
			return resolvedSelectedModel;
		}

		const resolvedChatModel = resolveModelOptionId(
			chatLastModelConfigID,
			modelOptions,
		);
		if (resolvedChatModel) {
			return resolvedChatModel;
		}

		return (
			getUsableDefaultModelIDForOrganization(
				models,
				modelOptions,
				chatOrganizationId,
			) ||
			modelOptions[0]?.id ||
			""
		);
	})();
	const hasModelOptions = modelOptions.length > 0;
	const hasResolvedModelData =
		!isModelDataPending && Boolean(modelsQuery.data) && !modelsQuery.error;
	const hasUnavailableHistoricalModel =
		hasResolvedModelData &&
		isUnavailableHistoricalModelID(chatLastModelConfigID, modelOptions);
	const hasUserFixableModelProviders = hasUserFixableProviders(modelCatalog);
	const unavailableModelNotice = hasUnavailableHistoricalModel
		? hasModelOptions
			? "The model used by this chat is not available. A usable model is selected for new messages."
			: hasUserFixableModelProviders
				? "The model used by this chat is not available. Add your API key in provider settings to enable models."
				: "The model used by this chat is not available. Generation is disabled because no usable model is available."
		: hasResolvedModelData && !hasModelOptions
			? hasUserFixableModelProviders
				? "No usable chat model is available. Add your API key in provider settings to enable models."
				: "No usable chat model is currently available. Generation is disabled."
			: undefined;

	const effectiveModelOption = modelOptions.find(
		(option) => option.id === effectiveSelectedModel,
	);
	const effectiveReasoningEffort = effectiveModelOption
		? pickReasoningEffort(
				selectedReasoningEffort || chat?.last_reasoning_effort,
				effectiveModelOption.reasoningEfforts ?? [],
				effectiveModelOption.reasoningEffortDefault,
			)
		: undefined;

	const modelSelectorPlaceholder = getModelSelectorPlaceholder(
		modelOptions,
		isModelDataPending,
		hasConfiguredModels,
		modelCatalog,
	);
	const modelSelectorHelp = getModelSelectorHelp({
		isModelCatalogLoading: isModelDataPending,
		hasModelOptions,
		hasConfiguredModels,
		hasUserFixableModelProviders,
	});
	const isSubmissionPending =
		isSendPending ||
		isEditPending ||
		isInterruptPending ||
		isCompactPending ||
		isClearPending;
	const isChatSettingsPending =
		isUpdateChatPlanModePending || isUpdateChatWorkspacePending;
	const isInputDisabled =
		!hasModelOptions ||
		isArchived ||
		isChatSettingsPending ||
		isViewerNotOwner ||
		aiGatewayDisabled;
	const canUpdateChatWorkspace = !isArchived && !isViewerNotOwner;
	const selectedWorkspaceId = chatQuery.data?.workspace_id ?? null;
	const handlePlanModeToggle = (enabled: boolean) => {
		if (enabled === planModeEnabled) {
			return;
		}
		trackPendingChatSettingSync(
			updateChatPlanModeAsync({
				chatId: agentId,
				planMode: enabled ? "plan" : undefined,
			}),
			pendingPlanModeSyncRef,
		);
	};

	const handleRequestError = (error: unknown): void => {
		if (!isApiError(error)) {
			return;
		}
		const detail = error.response?.data?.detail?.trim() || undefined;
		const kind = isChatHookDeniedResponse(error.response?.data)
			? "hook_denied"
			: isChatHookDispatchFailedResponse(error.response?.data)
				? "hook_dispatch_failed"
				: "generic";
		const reason: ChatDetailError = {
			kind,
			message: getErrorMessage(error, "An unexpected error occurred."),
			...(detail ? { detail } : {}),
		};
		store.setStreamError(reason);
		setChatErrorReason(agentId, reason);
	};

	const handleInterrupt = () => {
		if (isInterruptPending) {
			return;
		}
		void interrupt();
	};

	const handleWorkspaceChange = (nextWorkspaceId: string | null) => {
		if (nextWorkspaceId === selectedWorkspaceId) {
			return;
		}
		trackPendingChatSettingSync(
			updateChatWorkspaceAsync({
				chatId: agentId,
				workspaceId: nextWorkspaceId,
			}),
			pendingWorkspaceSyncRef,
		);
	};

	const handleDeleteQueuedMessage = async (id: number) => {
		const previousQueuedMessages = store.getSnapshot().queuedMessages;
		store.setQueuedMessages(
			previousQueuedMessages.filter((message) => message.id !== id),
		);
		try {
			await deleteQueuedMessage(id);
		} catch (error) {
			store.setQueuedMessages(previousQueuedMessages);
			throw error;
		}
	};

	const handlePromoteQueuedMessage = (id: number) =>
		runPromoteQueuedMessage({
			id,
			store,
			promoteQueuedMessage,
			agentId,
			clearChatErrorReason,
			onError: handleRequestError,
		});

	const editing = useConversationEditingState({
		chatID: agentId,
		onSend: handleSend,
		chatInputRef,
		inputValueRef,
	});
	const handleEditUserMessage = (
		...args: Parameters<typeof editing.handleEditUserMessage>
	) => {
		isEditReasoningEffortDirtyRef.current = false;
		editing.handleEditUserMessage(...args);
	};

	const chatTitle = chatQuery.data?.title;

	const sshCommand =
		workspace && workspaceAgent && sshConfigQuery.data?.hostname_suffix
			? `ssh ${workspaceAgent.name}.${workspace.name}.${workspace.owner_name}.${sshConfigQuery.data.hostname_suffix}`
			: undefined;

	// Signal ready only after the store has synced fetched messages,
	// so the DOM actually contains them when the parent scrolls.
	const chatReadyFiredRef = useRef<string | null>(null);
	const storeMessageCount = useChatSelector(store, (s) => s.messagesByID.size);
	const fetchedMessageCount = chatMessagesList?.length ?? 0;
	useEffect(() => {
		if (
			chatReadyFiredRef.current === agentId ||
			!chatMessagesQuery.isSuccess ||
			storeMessageCount < fetchedMessageCount
		) {
			return;
		}
		chatReadyFiredRef.current = agentId;
		onChatReady();
	}, [
		onChatReady,
		storeMessageCount,
		fetchedMessageCount,
		chatMessagesQuery.isSuccess,
		agentId,
	]);

	// Primitives extracted from proxy/workspace so the compiler
	// tracks stable strings, not object identity.
	const proxyHost = proxy.preferredWildcardHostname;
	const agentName = workspaceAgent?.name;
	const wsName = workspace?.name;
	const wsOwner = workspace?.owner_name;
	const urlTransform: UrlTransform = (url) => {
		if (!proxyHost || !agentName || !wsName || !wsOwner) {
			return url;
		}
		return rewriteLocalhostURL(url, proxyHost, agentName, wsName, wsOwner);
	};

	const chatTurnDeps = {
		isSubmissionPending,
		hasModelOptions,
		pendingPlanModeSyncRef,
		pendingWorkspaceSyncRef,
		isEditReasoningEffortDirtyRef,
		personalSkills: personalSkillsQuery.isSuccess
			? personalSkillsQuery.data
			: undefined,
		workspaceSkills: chatWorkspaceSkills,
		compact,
		clearChatContext,
		store,
		agentId,
		clearChatErrorReason,
		acceptServerChatStatus,
		chatMessages: chatMessagesList,
		effectiveSelectedModel,
		modelOptions,
		effectiveReasoningEffort,
		mcpServerIds: effectiveMCPServerIds,
		editMessage,
		sendMessage,
		onRequestError: handleRequestError,
		invalidateChat: (chatId: string) => {
			void invalidateChatEntity(queryClient, chatId);
		},
		scrollToEnd,
		upsertCacheMessages,
		getCacheQueuedMessages,
		setCacheQueuedMessages,
		fetchQueueConvergence: (chatId: string) =>
			queryClient.fetchQuery(chatQueueConvergence(chatId)),
		setCachedChatPlanMode,
	};

	async function handleSend(
		message: string,
		attachments?: readonly PendingAttachment[],
		editedMessageID?: number,
	) {
		await submitChatTurn({
			...chatTurnDeps,
			message,
			attachments,
			editedMessageID,
			composerParts: editing.chatInputRef.current?.getContentParts() ?? [],
		});
	}

	const handleSendAskUserQuestionResponse = async (message: string) => {
		await submitChatTurn({
			...chatTurnDeps,
			message,
		});
	};

	const handleImplementPlan = async () => {
		await submitChatTurn({
			...chatTurnDeps,
			message: "Implement the plan.",
			planModeSwitch: "clear",
		});
	};

	return (
		<>
			<title>
				{chatTitle ? pageTitle(chatTitle, "Agents") : pageTitle("Agents")}
			</title>
			{chatQuery.isLoading || chatMessagesQuery.isLoading ? (
				<AgentChatPageLoadingView
					inputRef={editing.chatInputRef}
					initialValue={editing.editorInitialValue}
					initialEditorState={editing.initialEditorState}
					remountKey={editing.remountKey}
					onContentChange={editing.handleLoadingDraftChange}
					isInputDisabled={isInputDisabled}
					effectiveSelectedModel={effectiveSelectedModel}
					setSelectedModel={setSelectedModel}
					modelOptions={modelOptions}
					modelSelectorPlaceholder={modelSelectorPlaceholder}
					hasModelOptions={hasModelOptions}
					isModelCatalogLoading={isModelDataPending}
					planModeEnabled={planModeEnabled}
					onPlanModeToggle={handlePlanModeToggle}
					showRightPanel={showSidebarPanel}
				/>
			) : chatQuery.isLoadingError || chatMessagesQuery.isLoadingError ? (
				getErrorStatus(chatQuery.error) === 404 ? (
					<AgentChatPageNotFoundView />
				) : (
					<AgentChatPageErrorView
						error={
							chatQuery.isLoadingError
								? chatQuery.error
								: chatMessagesQuery.error
						}
						onRetry={() => {
							if (chatQuery.isLoadingError) {
								void chatQuery.refetch();
							}
							if (chatMessagesQuery.isLoadingError) {
								void chatMessagesQuery.refetch();
							}
						}}
					/>
				)
			) : !chat || !chatMessagesQuery.data?.pages?.length ? (
				<AgentChatPageNotFoundView />
			) : (
				<AgentChatPageView
					key={agentId}
					chat={chat}
					persistedError={persistedError}
					workspace={workspace}
					workspaceAgent={workspaceAgent}
					store={store}
					initialMessages={chatMessagesList ?? []}
					editing={{ ...editing, handleEditUserMessage }}
					effectiveSelectedModel={effectiveSelectedModel}
					setSelectedModel={setSelectedModel}
					modelOptions={modelOptions}
					models={modelCatalog?.models}
					modelSelectorPlaceholder={modelSelectorPlaceholder}
					modelSelectorHelp={modelSelectorHelp}
					modelCatalogError={modelsQuery.error}
					unavailableModelNotice={unavailableModelNotice}
					reasoningEffort={effectiveReasoningEffort}
					onReasoningEffortChange={(value) => {
						setSelectedReasoningEffort(value);
						if (editing.editingMessageId !== null) {
							isEditReasoningEffortDirtyRef.current = true;
						}
					}}
					canConfigureAgentSetup={permissions.editDeploymentConfig}
					providerCount={providerCount}
					modelCount={modelCount}
					unsupportedProviderNames={unsupportedProviderNames}
					aiGatewayDisabled={aiGatewayDisabled}
					hasModelOptions={hasModelOptions}
					isModelCatalogLoading={isModelDataPending}
					onPlanModeToggle={handlePlanModeToggle}
					isInputDisabled={isInputDisabled}
					isSubmissionPending={isSubmissionPending}
					isInterruptPending={isInterruptPending}
					onWorkspaceChange={
						canUpdateChatWorkspace ? handleWorkspaceChange : undefined
					}
					isWorkspaceLoading={isUpdateChatWorkspacePending}
					showSidebarPanel={showSidebarPanel}
					onSetShowSidebarPanel={handleSetShowSidebarPanel}
					debugLoggingEnabled={debugLoggingEnabled}
					gitWatcher={gitWatcher}
					sshCommand={sshCommand}
					handleCommit={handleCommit}
					handleInterrupt={handleInterrupt}
					handleDeleteQueuedMessage={handleDeleteQueuedMessage}
					handlePromoteQueuedMessage={handlePromoteQueuedMessage}
					onImplementPlan={handleImplementPlan}
					onSendAskUserQuestionResponse={handleSendAskUserQuestionResponse}
					urlTransform={urlTransform}
					hasMoreMessages={Boolean(chatMessagesQuery.hasNextPage)}
					isFetchingMoreMessages={chatMessagesQuery.isFetchingNextPage}
					isHydratingMessages={isHydratingMessages}
					hasFetchMoreError={chatMessagesQuery.isFetchNextPageError}
					onFetchMoreMessages={chatMessagesQuery.fetchNextPage}
					desktopChatId={desktopEnabled ? agentId : undefined}
					mcpServers={mcpServers}
					selectedMCPServerIds={effectiveMCPServerIds}
					onMCPSelectionChange={handleMCPSelectionChange}
					onMCPAuthComplete={handleMCPAuthComplete}
				/>
			)}
		</>
	);
};

// Keyed so that navigating between agents (changing the :agentId param)
// fully remounts the component, resetting all internal state (drafts,
// editing, queries, scroller) cleanly.
const KeyedAgentChatPage: FC = () => {
	const { agentId } = useParams<{ agentId: string }>();
	if (!agentId) {
		return <AgentChatPageNotFoundView />;
	}
	return (
		<MessageScroller.Provider
			key={agentId}
			autoScroll
			defaultScrollPosition="end"
		>
			<AgentChatPage />
		</MessageScroller.Provider>
	);
};

export default KeyedAgentChatPage;
