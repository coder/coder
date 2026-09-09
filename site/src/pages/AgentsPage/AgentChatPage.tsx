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
import { checkAuthorization } from "#/api/queries/authCheck";
import { buildOptimisticEditedMessage } from "#/api/queries/chatMessageEdits";
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
	toChatPlanModePayload,
	updateChatPlanMode,
	updateChatWorkspace,
	updateInfiniteChatsCache,
	userChatDebugLogging,
} from "#/api/queries/chats";
import { deploymentSSHConfig } from "#/api/queries/deployment";
import { userSkills } from "#/api/queries/userSkills";
import {
	workspaceById,
	workspaceByIdKey,
	workspaces,
} from "#/api/queries/workspaces";
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
import {
	getParentChatID,
	getWorkspaceAgent,
} from "./components/ChatConversation/chatHelpers";
import {
	buildInactiveChatQueueReconciliation,
	reconcilePromotedQueueHead,
	restoreOptimisticRequestSnapshot,
	runPromoteQueuedMessage,
	settlePromotedQueueHead,
	submitEdit,
} from "./components/ChatConversation/chatQueueReconciliation";
import {
	selectChatStatus,
	useChatSelector,
	useChatStore,
} from "./components/ChatConversation/chatStore";
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
import { getWorkspaceOptionsWithLinkedWorkspace } from "./components/workspaceOptions";
import {
	BuiltInCommandPendingError,
	useConversationEditingState,
} from "./hooks/useConversationEditingState";
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
import {
	CHAT_SLASH_COMMANDS,
	CLEAR_SLASH_COMMAND,
	COMPACT_SLASH_COMMAND,
	chatSlashCommandTriggerText,
	resolveChatSlashCommandAvailability,
} from "./utils/slashCommands";

const lastModelConfigIDStorageKey = "agents.last-model-config-id";

const AGENT_BINDING_REPAIR_POLL_MS = 30_000;

const buildAttachmentMediaTypes = (
	attachments?: readonly PendingAttachment[],
): ReadonlyMap<string, string> | undefined => {
	if (!attachments?.length) {
		return undefined;
	}

	return new Map(
		attachments.map(({ fileId, mediaType }) => [fileId, mediaType]),
	);
};

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
	const workspacesQuery = useQuery(workspaces({ q: "owner:me", limit: 0 }));
	const workspaceOptions = getWorkspaceOptionsWithLinkedWorkspace(
		workspacesQuery.data?.workspaces ?? [],
		workspace,
		currentUser.id,
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
	const isRootChat = chat !== undefined && getParentChatID(chat) === undefined;
	const chatAuthorizationObject =
		chat !== undefined
			? {
					resource_type: "chat" as const,
					owner_id: chat.owner_id,
					organization_id: chat.organization_id,
				}
			: undefined;
	const chatAuthorizationChecks: TypesGen.AuthorizationRequest["checks"] = {};
	if (chatAuthorizationObject !== undefined && isRootChat) {
		chatAuthorizationChecks.canShareChat = {
			object: chatAuthorizationObject,
			action: "share",
		};
	}
	const chatAuthorizationQuery = useQuery({
		...checkAuthorization({ checks: chatAuthorizationChecks }),
		enabled: Object.keys(chatAuthorizationChecks).length > 0,
	});
	const canShareChat =
		isRootChat && Boolean(chatAuthorizationQuery.data?.canShareChat);
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
		mutate: updateChatWorkspaceMutate,
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
		mutate: updateChatPlanModeMutate,
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

	const aiGatewayDisabled = !useAIGatewayEnabled();
	const { scrollToEnd } = useMessageScroller();
	const {
		store,
		isHydratingMessages,
		acceptServerChatStatus,
		clearStreamError,
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

	const isWorkspaceLoading =
		workspacesQuery.isLoading || isUpdateChatWorkspacePending;
	const handlePlanModeToggle = (enabled: boolean) => {
		if (enabled === planModeEnabled) {
			return;
		}
		updateChatPlanModeMutate({
			chatId: agentId,
			planMode: enabled ? "plan" : undefined,
		});
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
		updateChatWorkspaceMutate({
			chatId: agentId,
			workspaceId: nextWorkspaceId,
		});
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

	function buildChatInputContent({
		message,
		attachments,
		useComposerContent = true,
	}: {
		message: string;
		attachments?: readonly PendingAttachment[];
		useComposerContent?: boolean;
	}): { content: TypesGen.ChatInputPart[]; hasContent: boolean } {
		const content: TypesGen.ChatInputPart[] = [];

		if (useComposerContent) {
			const chatInputHandle = (
				editing.chatInputRef as React.RefObject<ChatMessageInputRef | null>
			)?.current;
			const editorParts = chatInputHandle?.getContentParts() ?? [];

			// Walk the Lexical tree in document order so file-reference
			// parts appear at the correct position relative to the
			// surrounding text the user typed.
			for (const part of editorParts) {
				if (part.type === "text") {
					if (part.text.trim()) {
						content.push({ type: "text", text: part.text });
					}
				} else {
					const reference = part.reference;
					content.push({
						type: "file-reference",
						file_name: reference.fileName,
						start_line: reference.startLine,
						end_line: reference.endLine,
						content: reference.content,
					});
				}
			}

			if (content.length === 0 && message.trim()) {
				content.push({ type: "text", text: message });
			}
		} else if (message.trim()) {
			content.push({ type: "text", text: message });
		}

		if (attachments && attachments.length > 0) {
			for (const { fileId } of attachments) {
				content.push({ type: "file", file_id: fileId });
			}
		}

		return { content, hasContent: content.length > 0 };
	}

	async function submitChatTurn({
		message,
		attachments,
		editedMessageID,
		useComposerContent = true,
		clearPlanMode = false,
	}: {
		message: string;
		attachments?: readonly PendingAttachment[];
		editedMessageID?: number;
		useComposerContent?: boolean;
		clearPlanMode?: boolean;
	}) {
		const { content, hasContent } = buildChatInputContent({
			message,
			attachments,
			useComposerContent,
		});
		if (
			!hasContent ||
			isSubmissionPending ||
			isChatSettingsPending ||
			!hasModelOptions
		) {
			return;
		}

		// Built-ins only intercept new, text-only sends. A personal or workspace
		// skill with the same name takes precedence.
		const builtInCommand =
			editedMessageID === undefined &&
			content.length === 1 &&
			content[0].type === "text"
				? CHAT_SLASH_COMMANDS.find(
						(command) =>
							content[0].text?.trim() === chatSlashCommandTriggerText(command),
					)
				: undefined;
		const builtInCommandResolution = builtInCommand
			? resolveChatSlashCommandAvailability(
					builtInCommand,
					personalSkillsQuery.isSuccess ? personalSkillsQuery.data : undefined,
					chatWorkspaceSkills,
				)
			: undefined;
		if (builtInCommandResolution === "pending" && builtInCommand) {
			const triggerText = chatSlashCommandTriggerText(builtInCommand);
			toast.info(
				`Checking whether ${triggerText} is available. Try again in a moment.`,
			);
			throw new BuiltInCommandPendingError();
		}
		if (builtInCommandResolution === "available" && builtInCommand) {
			switch (builtInCommand.name) {
				case COMPACT_SLASH_COMMAND.name: {
					// Set running before awaiting so the worker's streamed waiting status cannot
					// be overwritten if it arrives before the POST resolves.
					const previousSnapshot = store.getSnapshot();
					clearChatErrorReason(agentId);
					clearStreamError();
					store.clearStreamState();
					store.setChatStatus("running");
					try {
						await compact();
					} catch (error) {
						restoreOptimisticRequestSnapshot(store, previousSnapshot);
						toast.error(getErrorMessage(error, "Failed to compact chat."));
						throw error;
					}
					return;
				}
				case CLEAR_SLASH_COMMAND.name:
					try {
						await clearChatContext();
					} catch (error) {
						toast.error(getErrorMessage(error, "Failed to clear chat."));
						throw error;
					}
					return;
			}
		}

		if (editedMessageID !== undefined) {
			const originalEditedMessage = chatMessagesList?.find(
				(existingMessage) => existingMessage.id === editedMessageID,
			);
			const originalModelConfigID = originalEditedMessage?.model_config_id;
			const pickerModelConfigID = effectiveSelectedModel || undefined;
			const originalIsSelectable =
				originalModelConfigID !== undefined &&
				modelOptions.some((option) => option.id === originalModelConfigID);
			const originalIsUnavailable = isUnavailableHistoricalModelID(
				originalModelConfigID,
				modelOptions,
			);
			// Use the picker fallback for an unavailable historical model.
			// Override a selectable model only after the user changes it.
			// Omit blank and nil references so the backend preserves the original.
			const editSelectedModelConfigID =
				pickerModelConfigID &&
				(originalIsUnavailable ||
					(originalIsSelectable &&
						pickerModelConfigID !== originalModelConfigID))
					? pickerModelConfigID
					: undefined;
			// Omit so the backend preserves the original effort.
			const request: TypesGen.EditChatMessageRequest = {
				content,
				model_config_id: editSelectedModelConfigID,
				reasoning_effort: isEditReasoningEffortDirtyRef.current
					? effectiveReasoningEffort
					: undefined,
				mcp_server_ids: [...effectiveMCPServerIds],
			};
			const optimisticMessage = originalEditedMessage
				? buildOptimisticEditedMessage({
						requestContent: request.content,
						originalMessage: originalEditedMessage,
						attachmentMediaTypes: buildAttachmentMediaTypes(attachments),
					})
				: undefined;
			const previousSnapshot = store.getSnapshot();
			clearChatErrorReason(agentId);
			clearStreamError();
			store.batch(() => {
				store.setQueuedMessages([]);
				store.setChatStatus("running");
				store.clearStreamState();
			});
			await submitEdit({
				editMessage,
				editArgs: {
					messageId: editedMessageID,
					optimisticMessage,
					req: request,
				},
				onError: (error) => {
					restoreOptimisticRequestSnapshot(store, previousSnapshot);
					handleRequestError(error);
					// Hook dispatch failures can park an idle chat in error before returning the request error.
					acceptServerChatStatus();
					void invalidateChatEntity(queryClient, agentId);
				},
			});
			scrollToEnd({ behavior: "smooth" });
			if (editSelectedModelConfigID) {
				localStorage.setItem(
					lastModelConfigIDStorageKey,
					editSelectedModelConfigID,
				);
			}
			return;
		}

		const selectedModelConfigID = effectiveSelectedModel || undefined;
		const request = {
			content,
			model_config_id: selectedModelConfigID,
			reasoning_effort: effectiveReasoningEffort,
			mcp_server_ids: [...effectiveMCPServerIds],
			...(clearPlanMode ? { plan_mode: toChatPlanModePayload(undefined) } : {}),
		};
		clearChatErrorReason(agentId);
		clearStreamError();

		// An errored-chat send may promote the queue head that existed when the request began.
		const queuedMessagesBeforeSend = store.getSnapshot().queuedMessages;
		const queueHeadIDBeforeSend = queuedMessagesBeforeSend[0]?.id;
		const statusVersionBeforeSend = store.getServerChatStatusVersion();

		// Don't clear stream state before the POST completes.
		// For queued sends the WebSocket status events handle
		// clearing; for non-queued sends we clear explicitly
		// below. Clearing eagerly causes a visible cutoff.
		let response: Awaited<ReturnType<typeof sendMessage>>;
		try {
			response = await sendMessage(request);
		} catch (error) {
			handleRequestError(error);
			// Hook dispatch failures can park an idle chat in error before returning the request error.
			acceptServerChatStatus();
			void invalidateChatEntity(queryClient, agentId);
			throw error;
		}
		const isActiveChat = store.getActiveChatID() === agentId;
		// Waiting for the WebSocket on non-queued sends leaves stale stream state visible.
		if (!response.queued && isActiveChat) {
			store.clearStreamState();
			// Optimistically set status to "running" so the
			// Thinking indicator appears immediately.
			// The server accepted the message (not queued),
			// so it will start processing. The WebSocket
			// status:running event no-ops via the
			// setChatStatus guard. If the server transitions
			// to error/pending instead, the WebSocket event
			// overrides this optimistic value.
			store.setChatStatus("running");
		}
		// Upsert the full batch because a queued send can insert a promoted head below
		// the highest cached ID, which a reconnect would skip.
		const insertedMessages =
			response.messages ?? (response.message ? [response.message] : []);
		if (insertedMessages.length > 0) {
			upsertCacheMessages(insertedMessages);
			if (isActiveChat) {
				store.upsertDurableMessages(insertedMessages);
			}
			if (response.queued) {
				const reconciledQueue = isActiveChat
					? reconcilePromotedQueueHead(
							store,
							insertedMessages,
							queueHeadIDBeforeSend,
							response.queued_message,
						)
					: buildInactiveChatQueueReconciliation(
							getCacheQueuedMessages(),
							queuedMessagesBeforeSend,
							insertedMessages,
							queueHeadIDBeforeSend,
							response.queued_message,
						);
				if (reconciledQueue) {
					setCacheQueuedMessages(reconciledQueue);
					// A promoted head starts a turn, but any server status received during the
					// request is newer and must win.
					if (
						isActiveChat &&
						store.getServerChatStatusVersion() === statusVersionBeforeSend
					) {
						store.clearStreamState();
						store.setChatStatus("running");
					}
					if (isActiveChat && queueHeadIDBeforeSend !== undefined) {
						void settlePromotedQueueHead(
							store,
							agentId,
							queueHeadIDBeforeSend,
							(chatID) => queryClient.fetchQuery(chatQueueConvergence(chatID)),
						).then((settled) => {
							if (settled) {
								setCacheQueuedMessages(settled);
							}
						});
					}
				}
			}
		}
		if (selectedModelConfigID) {
			localStorage.setItem(lastModelConfigIDStorageKey, selectedModelConfigID);
		} else {
			localStorage.removeItem(lastModelConfigIDStorageKey);
		}
		if (clearPlanMode) {
			setCachedChatPlanMode(agentId, undefined);
		}
	}

	async function handleSend(
		message: string,
		attachments?: readonly PendingAttachment[],
		editedMessageID?: number,
	) {
		await submitChatTurn({
			message,
			attachments,
			editedMessageID,
		});
	}

	const handleSendAskUserQuestionResponse = async (message: string) => {
		await submitChatTurn({
			message,
			useComposerContent: false,
		});
	};

	const handleImplementPlan = async () => {
		await submitChatTurn({
			message: "Implement the plan.",
			clearPlanMode: true,
			useComposerContent: false,
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
					canShareChat={canShareChat}
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
					workspaceOptions={workspaceOptions}
					onWorkspaceChange={
						canUpdateChatWorkspace ? handleWorkspaceChange : undefined
					}
					isWorkspaceLoading={isWorkspaceLoading}
					showSidebarPanel={showSidebarPanel}
					onSetShowSidebarPanel={handleSetShowSidebarPanel}
					debugLoggingEnabled={debugLoggingEnabled}
					gitWatcher={gitWatcher}
					sshCommand={sshCommand}
					handleCommit={handleCommit}
					handleInterrupt={handleInterrupt}
					handleDeleteQueuedMessage={handleDeleteQueuedMessage}
					handlePromoteQueuedMessage={handlePromoteQueuedMessage}
					onImplementPlan={
						isChatSettingsPending ? undefined : handleImplementPlan
					}
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
