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
import {
	type ChatPlanModeOrClear,
	type CreateChatMessageRequestWithClearablePlanMode,
	watchWorkspace,
} from "#/api/api";
import { getErrorMessage, isApiError } from "#/api/errors";
import { buildOptimisticEditedMessage } from "#/api/queries/chatMessageEdits";
import {
	chat,
	chatDesktopEnabled,
	chatKey,
	chatMessagesForInfiniteScroll,
	chatModelConfigs,
	chatModels,
	createChatMessage,
	deleteChatQueuedMessage,
	editChatMessage,
	interruptChat,
	mcpServerConfigs,
	promoteChatQueuedMessage,
	updateChatPlanMode,
	updateChatWorkspace,
	updateInfiniteChatsCache,
	userChatDebugLogging,
	userCompactionThresholds,
} from "#/api/queries/chats";
import { deploymentSSHConfig } from "#/api/queries/deployment";
import { user as userQuery } from "#/api/queries/users";
import {
	workspaceById,
	workspaceByIdKey,
	workspaces,
} from "#/api/queries/workspaces";
import type * as TypesGen from "#/api/typesGenerated";
import type { ChatMessagePart } from "#/api/typesGenerated";
import { useProxy } from "#/contexts/ProxyContext";
import { useAuthenticated } from "#/hooks/useAuthenticated";
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
import { normalizeChatErrorPayload } from "./components/ChatConversation/chatError";
import {
	getParentChatID,
	getWorkspaceAgent,
} from "./components/ChatConversation/chatHelpers";
import {
	type ChatStore,
	type ChatStoreState,
	selectChatStatus,
	useChatSelector,
	useChatStore,
} from "./components/ChatConversation/chatStore";
import { useWorkspaceCreationWatcher } from "./components/ChatConversation/useWorkspaceCreationWatcher";
import type { PendingAttachment } from "./components/ChatPageContent";
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
/** @internal localStorage key prefix for the per-chat active sidebar tab. Exported for testing. */
export const lastActiveSidebarTabStorageKeyPrefix = "agents.last-active-tab.";

const clearChatPlanMode = "" satisfies ChatPlanModeOrClear;

type PlanModeSwitch = TypesGen.ChatPlanMode | "clear";

/**
 * Read the persisted plain-text draft for a given chat ID.
 * Returns the text portion of the draft (stripping Lexical JSON
 * wrapper if present) for backward compatibility.
 */
export function getPersistedDraftInputValue(
	chatID: string | undefined,
): string {
	if (!chatID) {
		return "";
	}
	return parseStoredDraft(
		localStorage.getItem(`${draftInputStorageKeyPrefix}${chatID}`),
	).text;
}

/**
 * Read the persisted active sidebar tab ID for a given chat. Returns
 * `null` when no value is stored or the chat ID is missing.
 */
export function getPersistedSidebarTabId(
	chatID: string | undefined,
): string | null {
	if (!chatID) {
		return null;
	}
	return localStorage.getItem(
		`${lastActiveSidebarTabStorageKeyPrefix}${chatID}`,
	);
}

/**
 * Persist the active sidebar tab ID for a given chat so it can be
 * restored across session switches. No-op when the chat ID is missing.
 */
export function savePersistedSidebarTabId(
	chatID: string | undefined,
	tabID: string,
): void {
	if (!chatID) {
		return;
	}
	localStorage.setItem(
		`${lastActiveSidebarTabStorageKeyPrefix}${chatID}`,
		tabID,
	);
}

/**
 * Remove the persisted active sidebar tab ID for a given chat. Called
 * when a chat is archived so a future unarchive starts fresh.
 */
export function clearPersistedSidebarTabId(chatID: string | undefined): void {
	if (!chatID) {
		return;
	}
	localStorage.removeItem(`${lastActiveSidebarTabStorageKeyPrefix}${chatID}`);
}

/** @internal Exported for testing. */
export const restoreOptimisticRequestSnapshot = (
	store: Pick<
		ChatStore,
		| "batch"
		| "setChatStatus"
		| "setQueuedMessages"
		| "setStreamError"
		| "setStreamState"
	>,
	snapshot: Pick<
		ChatStoreState,
		"chatStatus" | "queuedMessages" | "streamError" | "streamState"
	>,
): void => {
	store.batch(() => {
		store.setQueuedMessages(snapshot.queuedMessages);
		store.setChatStatus(snapshot.chatStatus);
		store.setStreamState(snapshot.streamState);
		store.setStreamError(snapshot.streamError);
	});
};

export async function submitEditAndScroll({
	editMessage,
	editArgs,
	scrollToBottom,
	onError,
}: {
	editMessage: (args: {
		messageId: number;
		optimisticMessage?: TypesGen.ChatMessage;
		req: TypesGen.EditChatMessageRequest;
	}) => Promise<unknown>;
	editArgs: {
		messageId: number;
		optimisticMessage?: TypesGen.ChatMessage;
		req: TypesGen.EditChatMessageRequest;
	};
	scrollToBottom: (() => void) | null | undefined;
	onError: (error: unknown) => void;
}): Promise<void> {
	try {
		await editMessage(editArgs);
	} catch (error) {
		onError(error);
		throw error;
	}
	// Scroll after the mutation resolves so the optimistic
	// truncation and server reconciliation have already been
	// applied to the DOM. Scrolling before this point causes
	// the sticky user message to cycle through prior messages
	// as the IntersectionObserver reacts to rapid layout
	// shifts between the old and truncated content.
	scrollToBottom?.();
}

/** @internal Exported for testing. */
export const waitForPendingChatSettingsSyncs = async (
	pendingSyncs: readonly (Promise<unknown> | null | undefined)[],
): Promise<void> => {
	const activeSyncs = pendingSyncs.filter(
		(pendingSync): pendingSync is Promise<unknown> =>
			pendingSync !== null && pendingSync !== undefined,
	);
	if (activeSyncs.length === 0) {
		return;
	}
	await Promise.all(activeSyncs);
};

/** @internal Exported for testing. */
export const filterWorkspaceOptionsByOrganization = (
	workspaceOptions: readonly TypesGen.Workspace[],
	organizationID: string | undefined,
): readonly TypesGen.Workspace[] => {
	if (!organizationID) {
		return [];
	}
	return workspaceOptions.filter(
		(workspace) => workspace.organization_id === organizationID,
	);
};

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

/** @internal Exported for testing. */
export function useConversationEditingState(deps: {
	chatID: string | undefined;
	onSend: (
		message: string,
		attachments?: readonly PendingAttachment[],
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
	const serializedEditorStateRef = useRef<string | undefined>(
		initialEditorState,
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
		serializedEditorStateRef.current = undefined;
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
		serializedEditorStateRef.current = savedState;
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
		serializedEditorStateRef.current = undefined;
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
		serializedEditorStateRef.current = savedState;
		setRemountKey((k) => k + 1);
		inputValueRef.current = savedText;
		setEditingQueuedMessageID(null);
		setDraftBeforeQueueEdit(null);
		setEditingFileBlocks([]);
	};

	// Clears the composer for an in-flight history edit and
	// returns a rollback function that restores the editing draft
	// if the send fails.
	const clearInputForHistoryEdit = (message: string) => {
		const snapshot = {
			editorState: serializedEditorStateRef.current,
			fileBlocks: editingFileBlocks,
			messageId: editingMessageId,
		};

		chatInputRef.current?.clear();
		inputValueRef.current = "";
		setEditingMessageId(null);

		return () => {
			setDraftState({
				editorInitialValue: message,
				initialEditorState: snapshot.editorState,
			});
			serializedEditorStateRef.current = snapshot.editorState;
			setRemountKey((k) => k + 1);
			inputValueRef.current = message;
			setEditingMessageId(snapshot.messageId);
			setEditingFileBlocks(snapshot.fileBlocks);
		};
	};

	// Clears all input and editing state after a successful send.
	const finalizeSuccessfulSend = (
		editedMessageID: number | undefined,
		queueEditID: number | null,
	) => {
		chatInputRef.current?.clear();
		if (!isMobileViewport()) {
			chatInputRef.current?.focus();
		}
		inputValueRef.current = "";
		serializedEditorStateRef.current = undefined;
		if (draftStorageKey) {
			localStorage.removeItem(draftStorageKey);
		}
		if (editedMessageID !== undefined) {
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

	// Wraps the parent onSend to clear local input/editing state
	// and handle queue-edit deletion.
	const handleSendFromInput = async (
		message: string,
		attachments?: readonly PendingAttachment[],
	) => {
		const editedMessageID =
			editingMessageId !== null ? editingMessageId : undefined;
		const queueEditID = editingQueuedMessageID;
		const sendPromise = onSend(message, attachments, editedMessageID);

		// For history edits, clear input immediately and prepare
		// a rollback in case the send fails.
		const rollback =
			editedMessageID !== undefined
				? clearInputForHistoryEdit(message)
				: undefined;

		try {
			await sendPromise;
		} catch (error) {
			rollback?.();
			throw error;
		}

		finalizeSuccessfulSend(editedMessageID, queueEditID);
	};

	const handleContentChange = (
		content: string,
		serializedEditorState: string,
		hasFileReferences: boolean,
	) => {
		inputValueRef.current = content;
		serializedEditorStateRef.current = serializedEditorState;

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
	if (chatStatus !== "error") {
		return undefined;
	}
	if (cachedError) {
		return cachedError;
	}
	return normalizeChatErrorPayload(chatRecord?.last_error);
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
	| "lifecycle_state"
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
	const { user: currentUser } = useAuthenticated();
	const [selectedModel, setSelectedModel] = useState("");
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
	const userDebugLoggingQuery = useQuery(userChatDebugLogging());
	const mcpServersQuery = useQuery(mcpServerConfigs());
	const workspacesQuery = useQuery(workspaces({ q: "owner:me", limit: 0 }));
	const workspaceOptions = filterWorkspaceOptionsByOrganization(
		workspacesQuery.data?.workspaces ?? [],
		chatQuery.data?.organization_id,
	);
	const desktopEnabled = desktopEnabledQuery.data?.enable_desktop ?? false;
	const debugLoggingEnabled =
		userDebugLoggingQuery.data?.debug_logging_enabled ?? false;

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
										nextAgent?.expanded_directory &&
									prevAgent?.lifecycle_state === nextAgent?.lifecycle_state
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

	const chatRecord = chatQuery.data;
	const isArchived = chatRecord?.archived ?? false;
	const isViewerNotOwner =
		chatRecord !== undefined && currentUser.id !== chatRecord.owner_id;
	const chatOwnerQuery = useQuery({
		...userQuery(chatRecord?.owner_id ?? ""),
		enabled: isViewerNotOwner && !isArchived,
	});
	const chatOwner =
		isViewerNotOwner && chatRecord !== undefined
			? {
					id: chatRecord.owner_id,
					...(chatOwnerQuery.data?.username
						? { username: chatOwnerQuery.data.username }
						: {}),
				}
			: undefined;
	const planModeEnabled = chatRecord?.plan_mode === "plan";

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
		// Collect all messages and deduplicate by ID.
		// Cross-page duplication can occur when upsertCacheMessages
		// writes a message into page 0 while the same ID still
		// exists in a later page. Last occurrence wins so the
		// most up-to-date content is preserved.
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
					has_more: chatMessagesQuery.data?.pages.at(-1)?.has_more ?? false,
				}
			: undefined;
	const isRegenerateTitleDisabled = isArchived || isRegeneratingThisChat;
	const chatLastModelConfigID = chatRecord?.last_model_config_id;

	// Destructure mutation results directly so the React Compiler
	// tracks stable primitives/functions instead of the whole result
	// object (TanStack Query v5 recreates it every render via object
	// spread). Keeping no intermediate variable prevents future code
	// from accidentally closing over the unstable object.
	const { isPending: isSendPending, mutateAsync: sendMessage } = useMutation(
		createChatMessage(queryClient, agentId ?? ""),
	);
	const { isPending: isEditPending, mutateAsync: editMessage } = useMutation(
		editChatMessage(queryClient, agentId ?? ""),
	);
	const { isPending: isInterruptPending, mutateAsync: interrupt } = useMutation(
		interruptChat(queryClient, agentId ?? ""),
	);
	const { mutateAsync: deleteQueuedMessage } = useMutation(
		deleteChatQueuedMessage(queryClient, agentId ?? ""),
	);
	const { mutateAsync: promoteQueuedMessage } = useMutation(
		promoteChatQueuedMessage(queryClient, agentId ?? ""),
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
		queryClient.setQueryData<TypesGen.Chat>(chatKey(chatId), (previousChat) =>
			previousChat ? { ...previousChat, plan_mode: planMode } : previousChat,
		);
	};

	const pendingPlanModeSyncRef = useRef<Promise<unknown> | null>(null);
	const pendingWorkspaceSyncRef = useRef<Promise<unknown> | null>(null);
	const trackPendingChatSettingSync = (
		syncPromise: Promise<unknown>,
		syncRef: { current: Promise<unknown> | null },
	) => {
		let trackedSync: Promise<unknown>;
		trackedSync = syncPromise.finally(() => {
			if (syncRef.current === trackedSync) {
				syncRef.current = null;
			}
		});
		syncRef.current = trackedSync;
		void trackedSync.catch(() => undefined);
	};

	const { store, clearStreamError, upsertCacheMessages } = useChatStore({
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
		isSendPending || isEditPending || isInterruptPending;
	const isChatSettingsPending =
		isUpdateChatPlanModePending || isUpdateChatWorkspacePending;
	const isInputDisabled =
		!hasModelOptions || isArchived || isChatSettingsPending;
	const selectedWorkspaceId = chatQuery.data?.workspace_id ?? null;

	const isWorkspaceLoading =
		workspacesQuery.isLoading || isUpdateChatWorkspacePending;
	const handlePlanModeToggle = (enabled: boolean) => {
		if (!agentId || enabled === planModeEnabled) {
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

	const handleInterrupt = () => {
		if (!agentId || isInterruptPending) {
			return;
		}
		void interrupt();
	};

	const handleWorkspaceChange = (nextWorkspaceId: string | null) => {
		if (!agentId || nextWorkspaceId === selectedWorkspaceId) {
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

	const handlePromoteQueuedMessage = async (id: number) => {
		const previousSnapshot = store.getSnapshot();
		store.setQueuedMessages(
			previousSnapshot.queuedMessages.filter((message) => message.id !== id),
		);
		store.clearStreamState();
		if (agentId) {
			clearChatErrorReason(agentId);
		}
		store.clearStreamError();
		store.setChatStatus("pending");
		try {
			const promotedMessage = await promoteQueuedMessage(id);
			// Insert the promoted message into the store and cache
			// immediately so it appears in the timeline without
			// waiting for the WebSocket to deliver it.
			store.upsertDurableMessage(promotedMessage);
			upsertCacheMessages([promotedMessage]);
		} catch (error) {
			restoreOptimisticRequestSnapshot(store, previousSnapshot);
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
	const sshCommand =
		workspace && workspaceAgent && sshConfigQuery.data?.hostname_suffix
			? `ssh ${workspaceAgent.name}.${workspace.name}.${workspace.owner_name}.${sshConfigQuery.data.hostname_suffix}`
			: undefined;

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
		planModeSwitch,
	}: {
		message: string;
		attachments?: readonly PendingAttachment[];
		editedMessageID?: number;
		useComposerContent?: boolean;
		planModeSwitch?: PlanModeSwitch;
	}) {
		const { content, hasContent } = buildChatInputContent({
			message,
			attachments,
			useComposerContent,
		});
		if (!hasContent || isSubmissionPending || !agentId || !hasModelOptions) {
			return;
		}
		// Wait for chat-setting mutations to settle before sending so the
		// message observes the workspace and plan-mode choices the user just made.
		await waitForPendingChatSettingsSyncs([
			pendingPlanModeSyncRef.current,
			pendingWorkspaceSyncRef.current,
		]);

		if (editedMessageID !== undefined) {
			const request: TypesGen.EditChatMessageRequest = { content };
			const originalEditedMessage = chatMessagesList?.find(
				(existingMessage) => existingMessage.id === editedMessageID,
			);
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
			await submitEditAndScroll({
				editMessage,
				editArgs: {
					messageId: editedMessageID,
					optimisticMessage,
					req: request,
				},
				scrollToBottom: scrollToBottomRef.current,
				onError: (error) => {
					restoreOptimisticRequestSnapshot(store, previousSnapshot);
					handleUsageLimitError(error);
				},
			});
			return;
		}

		const selectedModelConfigID = effectiveSelectedModel || undefined;
		const request: CreateChatMessageRequestWithClearablePlanMode = {
			content,
			model_config_id: selectedModelConfigID,
			mcp_server_ids:
				effectiveMCPServerIds.length > 0
					? [...effectiveMCPServerIds]
					: undefined,
			...(planModeSwitch !== undefined
				? {
						plan_mode:
							planModeSwitch === "clear" ? clearChatPlanMode : planModeSwitch,
					}
				: {}),
		};
		clearChatErrorReason(agentId);
		clearStreamError();
		scrollToBottomRef.current?.();

		// Don't clear stream state before the POST completes.
		// For queued sends the WebSocket status events handle
		// clearing; for non-queued sends we clear explicitly
		// below. Clearing eagerly causes a visible cutoff.
		let response: Awaited<ReturnType<typeof sendMessage>>;
		try {
			response = await sendMessage(request);
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
				upsertCacheMessages([response.message]);
			}
		}
		if (selectedModelConfigID) {
			localStorage.setItem(lastModelConfigIDStorageKey, selectedModelConfigID);
		} else {
			localStorage.removeItem(lastModelConfigIDStorageKey);
		}
		if (planModeSwitch !== undefined) {
			setCachedChatPlanMode(
				agentId,
				planModeSwitch === "clear" ? undefined : planModeSwitch,
			);
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

	const handleRegenerateTitle = () => {
		if (!agentId || isRegenerateTitleDisabled || !onRegenerateTitle) {
			return;
		}
		onRegenerateTitle(agentId);
	};

	const handleSendAskUserQuestionResponse = async (message: string) => {
		await submitChatTurn({
			message,
			useComposerContent: false,
		});
	};

	const handleImplementPlan = async () => {
		await submitChatTurn({
			message: "Implement the plan.",
			planModeSwitch: "clear",
			useComposerContent: false,
		});
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
				planModeEnabled={planModeEnabled}
				onPlanModeToggle={handlePlanModeToggle}
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
			organizationId={chatQuery.data?.organization_id}
			chatTitle={chatTitle}
			parentChat={parentChat}
			persistedError={persistedError}
			isArchived={isArchived}
			chatOwner={chatOwner}
			workspace={workspace}
			workspaceAgent={workspaceAgent}
			chatBuildId={chatQuery.data?.build_id}
			store={store}
			editing={editing}
			effectiveSelectedModel={effectiveSelectedModel}
			setSelectedModel={setSelectedModel}
			modelOptions={modelOptions}
			modelSelectorPlaceholder={modelSelectorPlaceholder}
			modelSelectorHelp={modelSelectorHelp}
			hasModelOptions={hasModelOptions}
			isModelCatalogLoading={isModelCatalogLoading}
			planModeEnabled={planModeEnabled}
			onPlanModeToggle={handlePlanModeToggle}
			compressionThreshold={compressionThreshold}
			isInputDisabled={isInputDisabled}
			isSubmissionPending={isSubmissionPending}
			isInterruptPending={isInterruptPending}
			workspaceOptions={workspaceOptions}
			selectedWorkspaceId={selectedWorkspaceId}
			onWorkspaceChange={handleWorkspaceChange}
			isWorkspaceLoading={isWorkspaceLoading}
			isSidebarCollapsed={isSidebarCollapsed}
			onToggleSidebarCollapsed={onToggleSidebarCollapsed}
			showSidebarPanel={showSidebarPanel}
			onSetShowSidebarPanel={handleSetShowSidebarPanel}
			prNumber={prNumber}
			diffStatusData={chatQuery.data?.diff_status}
			debugLoggingEnabled={debugLoggingEnabled}
			gitWatcher={gitWatcher}
			sshCommand={sshCommand}
			handleCommit={handleCommit}
			handleInterrupt={handleInterrupt}
			handleDeleteQueuedMessage={handleDeleteQueuedMessage}
			handlePromoteQueuedMessage={handlePromoteQueuedMessage}
			onImplementPlan={handleImplementPlan}
			onSendAskUserQuestionResponse={handleSendAskUserQuestionResponse}
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
			messageCount={storeMessageCount}
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
