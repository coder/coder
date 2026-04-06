import { type FC, useEffect, useLayoutEffect, useRef, useState } from "react";

import {
	useInfiniteQuery,
	useMutation,
	useQuery,
	useQueryClient,
} from "react-query";
import { useOutletContext, useParams } from "react-router";
import { toast } from "sonner";
import type { UrlTransform } from "streamdown";
import { API, watchWorkspace } from "#/api/api";
import { isApiError } from "#/api/errors";
import {
	chat,
	chatDesktopEnabled,
	chatMessagesForInfiniteScroll,
	chatModelConfigs,
	chatModels,
	createChatMessage,
	deleteChatQueuedMessage,
	editChatMessage,
	interruptChat,
	mcpServerConfigs,
	promoteChatQueuedMessage,
	userCompactionThresholds,
} from "#/api/queries/chats";
import { deploymentSSHConfig } from "#/api/queries/deployment";
import { workspaceById, workspaceByIdKey } from "#/api/queries/workspaces";
import type * as TypesGen from "#/api/typesGenerated";
import type { ChatMessagePart } from "#/api/typesGenerated";
import { useProxy } from "#/contexts/ProxyContext";
import {
	getTerminalHref,
	getVSCodeHref,
	openAppInNewWindow,
} from "#/modules/apps/apps";
import { isMobileViewport } from "#/utils/mobile";
import { pageTitle } from "#/utils/page";
import { rewriteLocalhostURL } from "#/utils/portForward";
import { createReconnectingWebSocket } from "#/utils/reconnectingWebSocket";
import {
	AgentChatPageLoadingView,
	AgentChatPageNotFoundView,
	AgentChatPageView,
} from "./AgentChatPageView";
import type { AgentsOutletContext } from "./AgentsPage";
import type { ChatMessageInputRef } from "./components/AgentChatInput";
import {
	getParentChatID,
	getWorkspaceAgent,
} from "./components/ChatConversation/chatHelpers";
import {
	selectChatStatus,
	useChatSelector,
	useChatStore,
} from "./components/ChatConversation/chatStore";
import { useWorkspaceCreationWatcher } from "./components/ChatConversation/useWorkspaceCreationWatcher";
import {
	getDefaultMCPSelection,
	getSavedMCPSelection,
	saveMCPSelection,
} from "./components/MCPServerPicker";
import { getModelSelectorHelp } from "./components/ModelSelectorHelp";
import { useGitWatcher } from "./hooks/useGitWatcher";
import { type ParsedDraft, parseStoredDraft } from "./utils/draftStorage";
import {
	getModelOptionsFromConfigs,
	getModelSelectorPlaceholder,
	hasConfiguredModelsInCatalog,
	hasUserFixableProviders,
	resolveModelOptionId,
} from "./utils/modelOptions";
import { parsePullRequestUrl } from "./utils/pullRequest";
import {
	type ChatDetailError,
	formatUsageLimitMessage,
	isUsageLimitData,
} from "./utils/usageLimitMessage";

/** localStorage key controlling whether the right panel is visible. */
export const RIGHT_PANEL_OPEN_KEY = "agents.right-panel-open";

const lastModelConfigIDStorageKey = "agents.last-model-config-id";
/** @internal Exported for testing. */
export const draftInputStorageKeyPrefix = "agents.draft-input.";

/**
 * Read the persisted plain-text draft for a given chat ID.
 * Returns the text portion of the draft (stripping Lexical JSON
 * wrapper if present) for backward compatibility.
 */
export function getPersistedDraftInputValue(
	chatID: string | undefined,
): string {
	if (typeof window === "undefined" || !chatID) {
		return "";
	}
	return parseStoredDraft(
		localStorage.getItem(`${draftInputStorageKeyPrefix}${chatID}`),
	).text;
}

/** @internal Exported for testing. */
export function useConversationEditingState(deps: {
	chatID: string | undefined;
	onSend: (
		message: string,
		fileIds?: string[],
		editedMessageID?: number,
	) => Promise<void>;
	onDeleteQueuedMessage: (id: number) => Promise<void>;
	chatInputRef: React.RefObject<ChatMessageInputRef | null>;
	inputValueRef: React.RefObject<string>;
}) {
	const { chatID, onSend, onDeleteQueuedMessage, chatInputRef, inputValueRef } =
		deps;
	const draftStorageKey = chatID
		? `${draftInputStorageKeyPrefix}${chatID}`
		: null;
	const [{ editorInitialValue, initialEditorState }, setDraftState] = useState(
		() => {
			if (typeof window === "undefined" || !draftStorageKey) {
				return { editorInitialValue: "", initialEditorState: undefined };
			}
			const draft = parseStoredDraft(localStorage.getItem(draftStorageKey));
			return {
				editorInitialValue: draft.text,
				initialEditorState: draft.editorState,
			};
		},
	);

	// Monotonic counter to force LexicalComposer remount.
	const [remountKey, setRemountKey] = useState(0);

	// Sync the ref with the initial draft value so callers that
	// read inputValueRef.current see the persisted draft. Uses a
	// layout effect so the value is available before paint.
	const initialSyncDone = useRef(false);
	useLayoutEffect(() => {
		if (!initialSyncDone.current && editorInitialValue) {
			initialSyncDone.current = true;
			(inputValueRef as React.MutableRefObject<string>).current =
				editorInitialValue;
		}
	}, [editorInitialValue, inputValueRef]);

	// -- History editing state --
	const [editingMessageId, setEditingMessageId] = useState<number | null>(null);
	const [draftBeforeHistoryEdit, setDraftBeforeHistoryEdit] =
		useState<ParsedDraft | null>(null);
	const [editingFileBlocks, setEditingFileBlocks] = useState<
		readonly ChatMessagePart[]
	>([]);

	const handleEditUserMessage = (
		messageId: number,
		text: string,
		fileBlocks?: readonly ChatMessagePart[],
	) => {
		if (editingMessageId === null) {
			// Read the current serialized editor state from localStorage
			// (kept up-to-date by handleContentChange) rather than from
			// the stale initialEditorState React state.
			const currentEditorState = draftStorageKey
				? parseStoredDraft(localStorage.getItem(draftStorageKey)).editorState
				: undefined;
			setDraftBeforeHistoryEdit({
				text: inputValueRef.current,
				editorState: currentEditorState,
			});
		}
		setEditingMessageId(messageId);
		setDraftState({
			editorInitialValue: text,
			initialEditorState: undefined,
		});
		setRemountKey((k) => k + 1);
		inputValueRef.current = text;
		setEditingFileBlocks(fileBlocks ?? []);
	};

	const handleCancelHistoryEdit = () => {
		const savedText = draftBeforeHistoryEdit?.text ?? "";
		const savedState = draftBeforeHistoryEdit?.editorState;
		setDraftState({
			editorInitialValue: savedText,
			initialEditorState: savedState,
		});
		setRemountKey((k) => k + 1);
		inputValueRef.current = savedText;
		setEditingMessageId(null);
		setDraftBeforeHistoryEdit(null);
		setEditingFileBlocks([]);
	};

	// -- Queue editing state --
	const [editingQueuedMessageID, setEditingQueuedMessageID] = useState<
		number | null
	>(null);
	const [draftBeforeQueueEdit, setDraftBeforeQueueEdit] =
		useState<ParsedDraft | null>(null);

	const handleStartQueueEdit = (
		id: number,
		text: string,
		fileBlocks: readonly ChatMessagePart[],
	) => {
		if (editingQueuedMessageID === null) {
			const currentEditorState = draftStorageKey
				? parseStoredDraft(localStorage.getItem(draftStorageKey)).editorState
				: undefined;
			setDraftBeforeQueueEdit({
				text: inputValueRef.current,
				editorState: currentEditorState,
			});
		}
		setEditingQueuedMessageID(id);
		setDraftState({
			editorInitialValue: text,
			initialEditorState: undefined,
		});
		setRemountKey((k) => k + 1);
		inputValueRef.current = text;
		setEditingFileBlocks(fileBlocks);
	};

	const handleCancelQueueEdit = () => {
		const savedText = draftBeforeQueueEdit?.text ?? "";
		const savedState = draftBeforeQueueEdit?.editorState;
		setDraftState({
			editorInitialValue: savedText,
			initialEditorState: savedState,
		});
		setRemountKey((k) => k + 1);
		inputValueRef.current = savedText;
		setEditingQueuedMessageID(null);
		setDraftBeforeQueueEdit(null);
		setEditingFileBlocks([]);
	};

	// Wraps the parent onSend to clear local input/editing state
	// and handle queue-edit deletion.
	const handleSendFromInput = async (message: string, fileIds?: string[]) => {
		const editedMessageID =
			editingMessageId !== null ? editingMessageId : undefined;
		const queueEditID = editingQueuedMessageID;

		await onSend(message, fileIds, editedMessageID);
		// Clear input and editing state on success.
		chatInputRef.current?.clear();
		if (!isMobileViewport()) {
			chatInputRef.current?.focus();
		}
		inputValueRef.current = "";
		if (draftStorageKey) {
			localStorage.removeItem(draftStorageKey);
		}
		if (editingMessageId !== null) {
			setEditingMessageId(null);
			setDraftBeforeHistoryEdit(null);
			setEditingFileBlocks([]);
		}
		if (queueEditID !== null) {
			setEditingQueuedMessageID(null);
			setDraftBeforeQueueEdit(null);
			setEditingFileBlocks([]);
			void onDeleteQueuedMessage(queueEditID);
		}
	};

	const handleContentChange = (
		content: string,
		serializedEditorState: string,
		hasFileReferences: boolean,
	) => {
		inputValueRef.current = content;

		// Don't overwrite the persisted draft while editing a
		// history or queued message — the original draft (possibly
		// containing file-reference chips) is saved in React state
		// and should survive a cancel.
		if (editingMessageId !== null || editingQueuedMessageID !== null) {
			return;
		}

		if (draftStorageKey) {
			const shouldPersist = content.trim() || hasFileReferences;
			if (shouldPersist) {
				try {
					localStorage.setItem(draftStorageKey, serializedEditorState);
				} catch {
					// QuotaExceededError — silently discard the draft.
				}
			} else {
				localStorage.removeItem(draftStorageKey);
			}
		}
	};

	return {
		inputValueRef,
		chatInputRef,
		editorInitialValue,
		initialEditorState,
		remountKey,
		editingMessageId,
		editingFileBlocks,
		handleEditUserMessage,
		handleCancelHistoryEdit,
		editingQueuedMessageID,
		handleStartQueueEdit,
		handleCancelQueueEdit,
		handleSendFromInput,
		handleContentChange,
	};
}

const getPersistedDetailError = ({
	chatStatus,
	chatRecord,
	cachedError,
}: {
	chatStatus: TypesGen.ChatStatus | null;
	chatRecord: TypesGen.Chat | undefined;
	cachedError: ChatDetailError | undefined;
}): ChatDetailError | undefined => {
	if (cachedError?.kind === "usage_limit") {
		return cachedError;
	}
	if (chatStatus === "error") {
		if (cachedError) {
			return cachedError;
		}
		const lastError = chatRecord?.last_error?.trim();
		if (lastError) {
			return { kind: "generic", message: lastError };
		}
	}
	return undefined;
};

/**
 * Resolves the effective compaction threshold for a model configuration,
 * preferring the user's override when set.
 */
function resolveCompactionThreshold(
	modelConfigID: string | undefined,
	userThresholds: readonly TypesGen.UserChatCompactionThreshold[] | undefined,
	modelConfigs: readonly TypesGen.ChatModelConfig[] | null | undefined,
): number | undefined {
	if (!modelConfigID || !Array.isArray(modelConfigs)) return undefined;
	const config = modelConfigs.find((c) => c.id === modelConfigID);
	if (!config) return undefined;
	const userOverride = userThresholds?.find(
		(threshold) => threshold.model_config_id === modelConfigID,
	);
	if (userOverride) {
		return userOverride.threshold_percent;
	}
	return config.compression_threshold;
}

// Compile-time guard: ensures the workspace watcher bailout comparison
// covers every WorkspaceAgent field the UI reads. If WorkspaceAgent
// gains a new field, this will error until the field is either added
// to the comparison or explicitly excluded here.
type _UncoveredAgentFields = Omit<
	TypesGen.WorkspaceAgent,
	| "id"
	| "status"
	| "name"
	| "expanded_directory"
	// Fields below are intentionally not compared. They change
	// frequently (stats, metadata) or are objects/arrays that would
	// require deep comparison, and the UI does not read them.
	| "parent_id"
	| "created_at"
	| "updated_at"
	| "first_connected_at"
	| "last_connected_at"
	| "disconnected_at"
	| "started_at"
	| "ready_at"
	| "lifecycle_state"
	| "resource_id"
	| "instance_id"
	| "architecture"
	| "environment_variables"
	| "operating_system"
	| "logs_length"
	| "logs_overflowed"
	| "directory"
	| "version"
	| "api_version"
	| "apps"
	| "latency"
	| "connection_timeout_seconds"
	| "troubleshooting_url"
	| "subsystems"
	| "health"
	| "display_apps"
	| "log_sources"
	| "scripts"
	| "startup_script_behavior"
>;
// If this errors, a new field was added to WorkspaceAgent.
// Decide: does the UI read it? If yes, add it to the first
// section of the Omit above and to the bailout comparison
// in the workspace watcher message handler. If no, add it
// to the excluded section of the Omit.
const _agentFieldGuard: Record<keyof _UncoveredAgentFields, true> = {};

const AgentChatPage: FC = () => {
	const { agentId } = useParams<{ agentId: string }>();
	const {
		chatErrorReasons,
		setChatErrorReason,
		clearChatErrorReason,
		requestArchiveAgent,
		requestArchiveAndDeleteWorkspace,
		requestUnarchiveAgent,
		onRegenerateTitle,
		regeneratingTitleChatIds,
		isSidebarCollapsed,
		onToggleSidebarCollapsed,
		onChatReady,
		scrollContainerRef,
	} = useOutletContext<AgentsOutletContext>();
	const queryClient = useQueryClient();
	const [selectedModel, setSelectedModel] = useState("");
	const [pendingEditMessageId, setPendingEditMessageId] = useState<
		number | null
	>(null);
	const scrollToBottomRef = useRef<(() => void) | null>(null);
	const chatInputRef = useRef<ChatMessageInputRef | null>(null);
	const inputValueRef = useRef(
		agentId
			? parseStoredDraft(
					localStorage.getItem(`${draftInputStorageKeyPrefix}${agentId}`),
				).text
			: "",
	);

	// Right panel open/closed state is owned here so the loading
	// skeleton and the loaded view share the same layout, preventing
	// a horizontal shift when data arrives.
	const [showSidebarPanel, setShowSidebarPanel] = useState(() => {
		return localStorage.getItem(RIGHT_PANEL_OPEN_KEY) === "true";
	});
	const handleSetShowSidebarPanel = (
		next: boolean | ((prev: boolean) => boolean),
	) => {
		setShowSidebarPanel((prev) => {
			const value = typeof next === "function" ? next(prev) : next;
			localStorage.setItem(RIGHT_PANEL_OPEN_KEY, String(value));
			return value;
		});
	};

	const isRegeneratingThisChat = agentId
		? regeneratingTitleChatIds.includes(agentId)
		: false;

	const chatQuery = useQuery({
		...chat(agentId ?? ""),
		enabled: Boolean(agentId),
	});
	const chatMessagesQuery = useInfiniteQuery({
		...chatMessagesForInfiniteScroll(agentId ?? ""),
		enabled: Boolean(agentId),
	});
	const parentChatID = getParentChatID(chatQuery.data);
	const parentChatQuery = useQuery({
		...chat(parentChatID ?? ""),
		enabled: Boolean(parentChatID),
	});
	const workspaceId = chatQuery.data?.workspace_id;
	const workspaceQuery = useQuery({
		...workspaceById(workspaceId ?? ""),
		enabled: Boolean(workspaceId),
	});

	const chatModelsQuery = useQuery(chatModels());
	const chatModelConfigsQuery = useQuery(chatModelConfigs());
	const userThresholdsQuery = useQuery(userCompactionThresholds());
	const desktopEnabledQuery = useQuery(chatDesktopEnabled());
	const mcpServersQuery = useQuery(mcpServerConfigs());
	const desktopEnabled = desktopEnabledQuery.data?.enable_desktop ?? false;

	// MCP server selection state.
	const mcpServers = mcpServersQuery.data ?? [];
	const [selectedMCPServerIds, setSelectedMCPServerIds] = useState<
		string[] | null
	>(null);

	const handleMCPSelectionChange = (ids: string[]) => {
		setSelectedMCPServerIds(ids);
		saveMCPSelection(ids);
	};

	const handleMCPAuthComplete = (_serverId: string) => {
		void mcpServersQuery.refetch();
	};

	const modelOptions = getModelOptionsFromConfigs(
		chatModelConfigsQuery.data,
		chatModelsQuery.data,
	);
	const modelConfigs = chatModelConfigsQuery.data ?? [];
	const modelCatalog = chatModelsQuery.data;
	const isModelCatalogLoading = chatModelsQuery.isLoading;

	// Subscribe to live workspace updates so that agent status changes
	// (e.g. connected/disconnected) are reflected without a page refresh.
	useEffect(() => {
		if (!workspaceId) {
			return;
		}
		return createReconnectingWebSocket({
			connect() {
				const socket = watchWorkspace(workspaceId);
				socket.addEventListener("message", (event) => {
					if (event.parseError) {
						return;
					}
					if (event.parsedMessage.type === "data") {
						const next = event.parsedMessage.data as TypesGen.Workspace;
						queryClient.setQueryData<TypesGen.Workspace | undefined>(
							workspaceByIdKey(workspaceId),
							(prev) => {
								// Return the same reference when nothing the UI
								// reads has changed. This prevents react-query
								// from notifying subscribers and avoids a full
								// AgentChatPage re-render on every heartbeat.
								const prevAgent = getWorkspaceAgent(prev, undefined);
								const nextAgent = getWorkspaceAgent(next, undefined);
								if (
									prev &&
									prev.latest_build.status === next.latest_build.status &&
									prev.health.healthy === next.health.healthy &&
									prev.name === next.name &&
									prev.owner_name === next.owner_name &&
									prevAgent?.id === nextAgent?.id &&
									prevAgent?.status === nextAgent?.status &&
									prevAgent?.name === nextAgent?.name &&
									prevAgent?.expanded_directory ===
										nextAgent?.expanded_directory
								) {
									return prev;
								}
								return next;
							},
						);
					}
				});
				return socket;
			},
			onOpen() {
				// Refetch workspace data on reconnection to cover
				// events missed while disconnected. Also fires on the
				// initial connection (harmless, may deduplicate with
				// the in-flight useQuery fetch).
				void queryClient.invalidateQueries({
					queryKey: workspaceByIdKey(workspaceId),
				});
			},
		});
	}, [workspaceId, queryClient]);
	const sshConfigQuery = useQuery(deploymentSSHConfig());
	const workspace = workspaceQuery.data;
	const workspaceAgent = getWorkspaceAgent(workspace, undefined);
	const { proxy } = useProxy();

	// Extract the primitive fields used by the transform so the
	// compiler can see the real dependencies and avoid invalidating
	// the closure when the workspace object reference changes but
	// the relevant fields haven't.
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

	const chatRecord = chatQuery.data;

	// Initialize MCP selection from chat record or defaults.
	const effectiveMCPServerIds = (() => {
		if (selectedMCPServerIds !== null) {
			return selectedMCPServerIds;
		}
		// If the chat has MCP server IDs recorded (even empty, meaning
		// the user deliberately opted out), use those.
		if (chatRecord?.mcp_server_ids) {
			return chatRecord.mcp_server_ids;
		}
		// Check for a previously saved selection in localStorage.
		const saved = getSavedMCPSelection(mcpServers);
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
		// Collect all messages, then sort chronologically by ID.
		const all = pages.flatMap((p) => p.messages);
		// Sort ascending by ID for chronological order.
		all.sort((a, b) => a.id - b.id);
		return all;
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
					has_more: chatMessagesQuery.data?.pages.at(-1)?.has_more ?? false,
				}
			: undefined;
	const isArchived = chatRecord?.archived ?? false;
	const isRegenerateTitleDisabled = isArchived || isRegeneratingThisChat;
	const chatLastModelConfigID = chatRecord?.last_model_config_id;

	const sendMutation = useMutation(
		createChatMessage(queryClient, agentId ?? ""),
	);
	const editMutation = useMutation(editChatMessage(queryClient, agentId ?? ""));
	const interruptMutation = useMutation(
		interruptChat(queryClient, agentId ?? ""),
	);
	const deleteQueuedMutation = useMutation(
		deleteChatQueuedMessage(queryClient, agentId ?? ""),
	);
	const promoteQueuedMutation = useMutation(
		promoteChatQueuedMessage(queryClient, agentId ?? ""),
	);

	const { store, clearStreamError } = useChatStore({
		chatID: agentId,
		chatMessages: chatMessagesList,
		chatRecord,
		chatMessagesData,
		chatQueuedMessages,
		setChatErrorReason,
		clearChatErrorReason,
	});
	const liveChatStatus =
		useChatSelector(store, selectChatStatus) ?? chatRecord?.status ?? null;
	const persistedError = getPersistedDetailError({
		chatStatus: liveChatStatus,
		chatRecord,
		cachedError: agentId ? chatErrorReasons[agentId] : undefined,
	});

	// Git watcher: runs regardless of sidebar visibility, but only
	// connects when the workspace agent is in the "connected" state
	// to avoid an infinite reconnect loop against a missing agent.
	const gitWatcher = useGitWatcher({
		chatId: agentId,
		agentStatus: workspaceAgent?.status,
	});

	// Detect workspace creation so the sidebar can resolve the
	// workspace and display agent/git info.
	useWorkspaceCreationWatcher({
		store,
		chatID: agentId,
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

	// Prefer the explicit PR number from the API, and only fall back to URL
	// parsing when older metadata does not provide it.
	const parsedPrNumber = Number(
		parsePullRequestUrl(chatQuery.data?.diff_status?.url)?.number,
	);
	const prNumber =
		chatQuery.data?.diff_status?.pr_number ?? (parsedPrNumber || undefined);
	// Compute an effective selected model by validating the user's
	// explicit choice against the current model options, falling
	// back to the chat's last model or the first available option.
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

		return modelOptions[0]?.id ?? "";
	})();

	const compressionThreshold = resolveCompactionThreshold(
		chatLastModelConfigID,
		userThresholdsQuery.data?.thresholds,
		modelConfigs,
	);
	const hasModelOptions = modelOptions.length > 0;
	const hasConfiguredModels = hasConfiguredModelsInCatalog(modelCatalog);
	const hasUserFixableModelProviders = hasUserFixableProviders(modelCatalog);
	const modelSelectorPlaceholder = getModelSelectorPlaceholder(
		modelOptions,
		isModelCatalogLoading,
		hasConfiguredModels,
		modelCatalog,
	);
	const modelSelectorHelp = getModelSelectorHelp({
		isModelCatalogLoading,
		hasModelOptions,
		hasConfiguredModels,
		hasUserFixableModelProviders,
	});
	const isSubmissionPending =
		sendMutation.isPending ||
		editMutation.isPending ||
		interruptMutation.isPending;
	const isInputDisabled = !hasModelOptions || isArchived;

	const handleUsageLimitError = (error: unknown): void => {
		if (!agentId) {
			return;
		}
		if (
			isApiError(error) &&
			error.response?.status === 409 &&
			isUsageLimitData(error.response.data)
		) {
			const reason: ChatDetailError = {
				kind: "usage_limit",
				message: formatUsageLimitMessage(error.response.data),
			};
			store.setStreamError(reason);
			setChatErrorReason(agentId, reason);
		} else if (isApiError(error)) {
			const reason: ChatDetailError = {
				kind: "generic",
				message: error.message || "An unexpected error occurred.",
			};
			store.setStreamError(reason);
			setChatErrorReason(agentId, reason);
		}
	};

	const handleSend = async (
		message: string,
		fileIds?: string[],
		editedMessageID?: number,
	) => {
		const chatInputHandle = (
			editing.chatInputRef as React.RefObject<ChatMessageInputRef | null>
		)?.current;

		// Walk the Lexical tree in document order so file-reference
		// parts appear at the correct position relative to the
		// surrounding text the user typed.
		const editorParts = chatInputHandle?.getContentParts() ?? [];
		const hasFileReferences = editorParts.some(
			(p) => p.type === "file-reference",
		);
		const hasContent =
			message.trim() || (fileIds && fileIds.length > 0) || hasFileReferences;
		if (!hasContent || isSubmissionPending || !agentId || !hasModelOptions) {
			return;
		}

		const content: TypesGen.ChatInputPart[] = [];

		// Emit parts in document order — text segments and
		// file-reference chips are interleaved as they appear in
		// the editor.
		for (const part of editorParts) {
			if (part.type === "text") {
				const trimmed = part.text.trim();
				if (trimmed) {
					content.push({ type: "text", text: part.text });
				}
			} else {
				const r = part.reference;
				content.push({
					type: "file-reference",
					file_name: r.fileName,
					start_line: r.startLine,
					end_line: r.endLine,
					content: r.content,
				});
			}
		}

		// Add pre-uploaded file references.
		if (fileIds && fileIds.length > 0) {
			for (const fileId of fileIds) {
				content.push({ type: "file", file_id: fileId });
			}
		}
		if (editedMessageID !== undefined) {
			const request: TypesGen.EditChatMessageRequest = { content };
			clearChatErrorReason(agentId);
			clearStreamError();
			setPendingEditMessageId(editedMessageID);
			scrollToBottomRef.current?.();
			try {
				await editMutation.mutateAsync({
					messageId: editedMessageID,
					req: request,
				});
				store.clearStreamState();
				store.setChatStatus("running");
				setPendingEditMessageId(null);
			} catch (error) {
				setPendingEditMessageId(null);
				handleUsageLimitError(error);
				throw error;
			}
			return;
		}
		const selectedModelConfigID = effectiveSelectedModel || undefined;
		const request: TypesGen.CreateChatMessageRequest = {
			content,
			model_config_id: selectedModelConfigID,
			mcp_server_ids:
				effectiveMCPServerIds.length > 0
					? [...effectiveMCPServerIds]
					: undefined,
		};
		clearChatErrorReason(agentId);
		clearStreamError();
		scrollToBottomRef.current?.();

		// Don't clear stream state before the POST completes.
		// For queued sends the WebSocket status events handle
		// clearing; for non-queued sends we clear explicitly
		// below. Clearing eagerly causes a visible cutoff.
		let response: Awaited<ReturnType<typeof sendMutation.mutateAsync>>;
		try {
			response = await sendMutation.mutateAsync(request);
		} catch (error) {
			handleUsageLimitError(error);
			throw error;
		}
		// When the server accepts the message immediately (not
		// queued), clear the stream and insert the user's message
		// so it appears in the timeline without waiting for the
		// WebSocket stream.
		if (!response.queued) {
			store.clearStreamState();
			// Optimistically set status to "running" so the
			// "Thinking..." indicator appears immediately.
			// The server accepted the message (not queued),
			// so it will start processing. The WebSocket
			// status:running event no-ops via the
			// setChatStatus guard. If the server transitions
			// to error/pending instead, the WebSocket event
			// overrides this optimistic value.
			store.setChatStatus("running");
			if (response.message) {
				store.upsertDurableMessage(response.message);
			}
		}
		if (selectedModelConfigID) {
			localStorage.setItem(lastModelConfigIDStorageKey, selectedModelConfigID);
		} else {
			localStorage.removeItem(lastModelConfigIDStorageKey);
		}
	};

	const handleInterrupt = () => {
		if (!agentId || interruptMutation.isPending) {
			return;
		}
		void interruptMutation.mutateAsync();
	};

	const handleDeleteQueuedMessage = async (id: number) => {
		const previousQueuedMessages = store.getSnapshot().queuedMessages;
		store.setQueuedMessages(
			previousQueuedMessages.filter((message) => message.id !== id),
		);
		try {
			await deleteQueuedMutation.mutateAsync(id);
		} catch (error) {
			store.setQueuedMessages(previousQueuedMessages);
			throw error;
		}
	};

	const handlePromoteQueuedMessage = async (id: number) => {
		const previousSnapshot = store.getSnapshot();
		const previousQueuedMessages = previousSnapshot.queuedMessages;
		const previousChatStatus = previousSnapshot.chatStatus;
		store.setQueuedMessages(
			previousQueuedMessages.filter((message) => message.id !== id),
		);
		store.clearStreamState();
		if (agentId) {
			clearChatErrorReason(agentId);
		}
		store.clearStreamError();
		store.setChatStatus("pending");
		try {
			const promotedMessage = await promoteQueuedMutation.mutateAsync(id);
			// Insert the promoted message into the store immediately
			// so it appears in the timeline without waiting for the
			// WebSocket to deliver it.
			store.upsertDurableMessage(promotedMessage);
		} catch (error) {
			store.setQueuedMessages(previousQueuedMessages);
			store.setChatStatus(previousChatStatus);
			handleUsageLimitError(error);
			throw error;
		}
	};

	const editing = useConversationEditingState({
		chatID: agentId,
		onSend: handleSend,
		onDeleteQueuedMessage: handleDeleteQueuedMessage,
		chatInputRef,
		inputValueRef,
	});

	const chatTitle = chatQuery.data?.title;

	const titleElement = (
		<title>
			{chatTitle ? pageTitle(chatTitle, "Agents") : pageTitle("Agents")}
		</title>
	);

	const parentChat = parentChatQuery.data;
	const workspaceRoute = workspace
		? `/@${workspace.owner_name}/${workspace.name}`
		: null;
	const canOpenWorkspace = Boolean(workspaceRoute);
	const canOpenEditors = Boolean(workspace && workspaceAgent);
	const terminalHref =
		workspace && workspaceAgent
			? getTerminalHref({
					username: workspace.owner_name,
					workspace: workspace.name,
					agent: workspaceAgent.name,
				})
			: null;
	const sshCommand =
		workspace && workspaceAgent && sshConfigQuery.data?.hostname_suffix
			? `ssh ${workspaceAgent.name}.${workspace.name}.${workspace.owner_name}.${sshConfigQuery.data.hostname_suffix}`
			: undefined;

	const generateKeyMutation = useMutation({
		mutationFn: () => API.getApiKey(),
	});

	const handleOpenInEditor = (editor: "cursor" | "vscode") => {
		if (!workspace || !workspaceAgent) {
			return;
		}

		// Prefer the active git repo root so VS Code opens to the
		// actual project directory, falling back to the agent's
		// configured directory.
		const repoRoots = Array.from(gitWatcher.repositories.keys()).sort();
		const folder = repoRoots[0] ?? workspaceAgent.expanded_directory;

		generateKeyMutation.mutate(undefined, {
			onSuccess: ({ key }) => {
				location.href = getVSCodeHref(editor, {
					owner: workspace.owner_name,
					workspace: workspace.name,
					token: key,
					agent: workspaceAgent.name,
					folder,
					chatId: agentId,
				});
			},
			onError: () => {
				toast.error(
					editor === "cursor"
						? "Failed to open in Cursor."
						: "Failed to open in VS Code.",
				);
			},
		});
	};

	const handleViewWorkspace = () => {
		if (!workspaceRoute) {
			return;
		}
		window.open(workspaceRoute, "_blank");
	};

	const handleOpenTerminal = () => {
		if (!terminalHref) {
			return;
		}
		openAppInNewWindow(terminalHref);
	};

	const handleArchiveAgentAction = () => {
		if (!agentId || isArchived) {
			return;
		}
		requestArchiveAgent(agentId);
	};

	const handleArchiveAndDeleteWorkspaceAction = () => {
		if (!agentId || isArchived || !workspaceId) {
			return;
		}
		requestArchiveAndDeleteWorkspace(agentId, workspaceId);
	};

	const handleUnarchiveAgentAction = () => {
		if (!agentId || !isArchived) {
			return;
		}
		requestUnarchiveAgent(agentId);
	};

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
		chatReadyFiredRef.current = agentId ?? null;
		onChatReady();
	}, [
		onChatReady,
		storeMessageCount,
		fetchedMessageCount,
		chatMessagesQuery.isSuccess,
		agentId,
	]);

	const handleRegenerateTitle = () => {
		if (!agentId || isRegenerateTitleDisabled || !onRegenerateTitle) {
			return;
		}
		onRegenerateTitle(agentId);
	};

	if (chatQuery.isLoading || chatMessagesQuery.isLoading) {
		return (
			<AgentChatPageLoadingView
				titleElement={titleElement}
				isInputDisabled={isInputDisabled}
				effectiveSelectedModel={effectiveSelectedModel}
				setSelectedModel={setSelectedModel}
				modelOptions={modelOptions}
				modelSelectorPlaceholder={modelSelectorPlaceholder}
				hasModelOptions={hasModelOptions}
				isModelCatalogLoading={isModelCatalogLoading}
				isSidebarCollapsed={isSidebarCollapsed}
				onToggleSidebarCollapsed={onToggleSidebarCollapsed}
				showRightPanel={showSidebarPanel}
			/>
		);
	}

	if (!chatQuery.data || !chatMessagesQuery.data?.pages?.length || !agentId) {
		return (
			<AgentChatPageNotFoundView
				titleElement={titleElement}
				isSidebarCollapsed={isSidebarCollapsed}
				onToggleSidebarCollapsed={onToggleSidebarCollapsed}
			/>
		);
	}

	return (
		<AgentChatPageView
			agentId={agentId}
			chatTitle={chatTitle}
			parentChat={parentChat}
			persistedError={persistedError}
			isArchived={isArchived}
			workspace={workspace}
			workspaceAgent={workspaceAgent}
			store={store}
			editing={editing}
			pendingEditMessageId={pendingEditMessageId}
			effectiveSelectedModel={effectiveSelectedModel}
			setSelectedModel={setSelectedModel}
			modelOptions={modelOptions}
			modelSelectorPlaceholder={modelSelectorPlaceholder}
			modelSelectorHelp={modelSelectorHelp}
			hasModelOptions={hasModelOptions}
			isModelCatalogLoading={isModelCatalogLoading}
			compressionThreshold={compressionThreshold}
			isInputDisabled={isInputDisabled}
			isSubmissionPending={isSubmissionPending}
			isInterruptPending={interruptMutation.isPending}
			isSidebarCollapsed={isSidebarCollapsed}
			onToggleSidebarCollapsed={onToggleSidebarCollapsed}
			showSidebarPanel={showSidebarPanel}
			onSetShowSidebarPanel={handleSetShowSidebarPanel}
			prNumber={prNumber}
			diffStatusData={chatQuery.data?.diff_status}
			gitWatcher={gitWatcher}
			canOpenEditors={canOpenEditors}
			canOpenWorkspace={canOpenWorkspace}
			sshCommand={sshCommand}
			handleOpenInEditor={handleOpenInEditor}
			handleViewWorkspace={handleViewWorkspace}
			handleOpenTerminal={handleOpenTerminal}
			handleCommit={handleCommit}
			handleInterrupt={handleInterrupt}
			handleDeleteQueuedMessage={handleDeleteQueuedMessage}
			handlePromoteQueuedMessage={handlePromoteQueuedMessage}
			handleArchiveAgentAction={handleArchiveAgentAction}
			handleUnarchiveAgentAction={handleUnarchiveAgentAction}
			handleArchiveAndDeleteWorkspaceAction={
				handleArchiveAndDeleteWorkspaceAction
			}
			handleRegenerateTitle={handleRegenerateTitle}
			isRegeneratingTitle={isRegeneratingThisChat}
			isRegenerateTitleDisabled={isRegenerateTitleDisabled}
			urlTransform={urlTransform}
			scrollContainerRef={scrollContainerRef}
			scrollToBottomRef={scrollToBottomRef}
			hasMoreMessages={chatMessagesQuery.hasNextPage ?? false}
			isFetchingMoreMessages={chatMessagesQuery.isFetchingNextPage}
			onFetchMoreMessages={chatMessagesQuery.fetchNextPage}
			desktopChatId={desktopEnabled ? agentId : undefined}
			mcpServers={mcpServers}
			selectedMCPServerIds={effectiveMCPServerIds}
			onMCPSelectionChange={handleMCPSelectionChange}
			onMCPAuthComplete={handleMCPAuthComplete}
			lastInjectedContext={chatQuery.data?.last_injected_context}
		/>
	);
};

// Keyed wrapper so that navigating between agents (changing the
// :agentId param) fully remounts the component, resetting all
// internal state — drafts, editing, queries — cleanly.
const KeyedAgentChatPage: FC = () => {
	const { agentId } = useParams<{ agentId: string }>();
	return <AgentChatPage key={agentId} />;
};

export default KeyedAgentChatPage;
