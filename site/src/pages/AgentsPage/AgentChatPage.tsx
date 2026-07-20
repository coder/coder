import {
	type FC,
	useEffect,
	useEffectEvent,
	useLayoutEffect,
	useRef,
	useState,
} from "react";

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
import { checkAuthorization } from "#/api/queries/authCheck";
import { buildOptimisticEditedMessage } from "#/api/queries/chatMessageEdits";
import {
	chat,
	chatKey,
	chatMessagesForInfiniteScroll,
	chatModelConfigs,
	chatModels,
	chatProviderConfigs,
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
	userChatProviderConfigs,
	userCompactionThresholds,
} from "#/api/queries/chats";
import { deploymentSSHConfig } from "#/api/queries/deployment";
import { preferenceSettings } from "#/api/queries/users";
import {
	workspaceById,
	workspaceByIdKey,
	workspaces,
} from "#/api/queries/workspaces";
import type * as TypesGen from "#/api/typesGenerated";
import type { ChatMessagePart } from "#/api/typesGenerated";
import { useProxy } from "#/contexts/ProxyContext";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useAIGatewayEnabled } from "#/hooks/useEmbeddedMetadata";
import {
	getDefaultOrganizationName,
	useDashboard,
} from "#/modules/dashboard/useDashboard";
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
import { useChatToolInvalidations } from "./components/ChatConversation/useChatToolInvalidations";
import type { PendingAttachment } from "./components/ChatPageContent";
import { workspaceSkillsFromChat } from "./components/ChatPageContent";
import {
	getDefaultMCPSelection,
	getSavedMCPSelection,
	saveMCPSelection,
} from "./components/MCPServerPicker";
import { getModelSelectorHelp } from "./components/ModelSelectorHelp";
import { useGitWatcher } from "./hooks/useGitWatcher";
import { getAgentChatSendShortcut } from "./utils/agentChatSendShortcut";
import { type ParsedDraft, parseStoredDraft } from "./utils/draftStorage";
import {
	countConfiguredProviderConfigs,
	getModelSelectorPlaceholder,
	getUnsupportedProviderNames,
	hasUserFixableProviders,
	resolveModelOptionId,
	resolveModelSelector,
} from "./utils/modelOptions";
import { parsePullRequestUrl } from "./utils/pullRequest";
import { pickReasoningEffort } from "./utils/reasoningEffort";
import {
	type ChatDetailError,
	formatUsageLimitMessage,
	isChatUsageLimitExceededResponse,
} from "./utils/usageLimitMessage";

/** localStorage key controlling whether the right panel is visible. */
export const RIGHT_PANEL_OPEN_KEY = "agents.right-panel-open";

const lastModelConfigIDStorageKey = "agents.last-model-config-id";
/** @internal Exported for testing. */
export const draftInputStorageKeyPrefix = "agents.draft-input.";

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

/**
 * Runs the optimistic queued-message promotion flow.
 *
 * The promote endpoint returns 202 Accepted with no message body, so the
 * actual user message is delivered via SSE or the messages REST endpoint.
 * Suppress the promoted ID so the transient reordered queue published by
 * the running-case backend does not flash the message back into the
 * visible queue. Roll back queue, status, and suppression on API error.
 *
 * @internal Exported for testing.
 */
export const runPromoteQueuedMessage = async (params: {
	id: number;
	store: Pick<
		ChatStore,
		| "batch"
		| "clearStreamError"
		| "clearStreamState"
		| "getSnapshot"
		| "setChatStatus"
		| "setQueuedMessages"
		| "setStreamError"
		| "setStreamState"
		| "suppressQueuedMessageID"
		| "unsuppressQueuedMessageID"
	>;
	promoteQueuedMessage: (id: number) => Promise<void>;
	agentId: string | undefined;
	clearChatErrorReason: (chatID: string) => void;
	handleUsageLimitError: (error: unknown) => void;
}): Promise<void> => {
	const {
		id,
		store,
		promoteQueuedMessage,
		agentId,
		clearChatErrorReason,
		handleUsageLimitError,
	} = params;
	const previousSnapshot = store.getSnapshot();
	store.batch(() => {
		store.suppressQueuedMessageID(id);
		store.setQueuedMessages(
			previousSnapshot.queuedMessages.filter((message) => message.id !== id),
		);
		store.clearStreamState();
		store.clearStreamError();
		store.setChatStatus("running");
	});
	if (agentId) {
		clearChatErrorReason(agentId);
	}
	try {
		await promoteQueuedMessage(id);
	} catch (error) {
		store.unsuppressQueuedMessageID(id);
		restoreOptimisticRequestSnapshot(store, previousSnapshot);
		handleUsageLimitError(error);
		throw error;
	}
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

/** @internal Exported for testing. */
export const getWorkspaceOptionsWithLinkedWorkspace = (
	workspaceOptions: readonly TypesGen.Workspace[],
	workspace: TypesGen.Workspace | undefined,
	ownerID: string,
): readonly TypesGen.Workspace[] => {
	if (!workspace || workspace.owner_id !== ownerID) {
		return workspaceOptions;
	}

	const existingIndex = workspaceOptions.findIndex(
		(candidate) => candidate.id === workspace.id,
	);
	if (existingIndex === -1) {
		return [workspace, ...workspaceOptions];
	}

	if (workspaceOptions[existingIndex] === workspace) {
		return workspaceOptions;
	}

	const nextWorkspaceOptions = [...workspaceOptions];
	nextWorkspaceOptions[existingIndex] = workspace;
	return nextWorkspaceOptions;
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
			if (!draftStorageKey) {
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

	// Separate from handleContentChange, which avoids setState to prevent
	// per-keystroke re-renders. The loading editor is a different instance
	// that unmounts on load, so the seed must advance here.
	const handleLoadingDraftChange = (
		content: string,
		serializedEditorState: string,
		hasFileReferences: boolean,
	) => {
		handleContentChange(content, serializedEditorState, hasFileReferences);
		setDraftState({
			editorInitialValue: content,
			initialEditorState: serializedEditorState,
		});
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
		handleLoadingDraftChange,
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
		requestPinAgent,
		requestUnpinAgent,
		isArchiving,
		archivingChatId,
		onOpenRenameDialog,
		isSidebarCollapsed,
		onToggleSidebarCollapsed,
		onChatReady,
		scrollContainerRef,
	} = useOutletContext<AgentsOutletContext>();
	const queryClient = useQueryClient();
	const { permissions, user: currentUser } = useAuthenticated();
	const { organizations, experiments } = useDashboard();
	const organizationName = getDefaultOrganizationName(organizations);
	const [selectedModel, setSelectedModel] = useState("");
	const [selectedReasoningEffort, setSelectedReasoningEffort] = useState("");
	const isEditReasoningEffortDirtyRef = useRef(false);
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
	const chatAgentId = chatQuery.data?.agent_id;
	const workspaceQuery = useQuery({
		...workspaceById(workspaceId ?? ""),
		enabled: Boolean(workspaceId),
	});
	const workspace = workspaceQuery.data;

	const chatModelsQuery = useQuery(chatModels());
	const chatModelConfigsQuery = useQuery(chatModelConfigs());
	const chatProviderConfigsQuery = useQuery({
		...chatProviderConfigs(),
		enabled: permissions.editDeploymentConfig,
	});
	const userProviderConfigsQuery = useQuery(userChatProviderConfigs());
	const userThresholdsQuery = useQuery(userCompactionThresholds());
	const preferencesQuery = useQuery(preferenceSettings());
	const userDebugLoggingQuery = useQuery(userChatDebugLogging());
	const mcpServersQuery = useQuery(mcpServerConfigs());
	const workspacesQuery = useQuery(workspaces({ q: "owner:me", limit: 0 }));
	const workspaceOptions = getWorkspaceOptionsWithLinkedWorkspace(
		workspacesQuery.data?.workspaces ?? [],
		workspace,
		currentUser.id,
	);
	const desktopEnabled = experiments.includes("chat-virtual-desktop");
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

	const {
		options: modelOptions,
		isModelCatalogLoading,
		modelCatalog,
		hasConfiguredModels,
	} = resolveModelSelector(
		chatModelConfigsQuery,
		chatModelsQuery,
		userProviderConfigsQuery,
	);
	const modelConfigs = chatModelConfigsQuery.data ?? [];
	const providerCount =
		permissions.editDeploymentConfig &&
		chatProviderConfigsQuery.isSuccess &&
		chatModelsQuery.isSuccess
			? countConfiguredProviderConfigs(
					chatProviderConfigsQuery.data,
					chatModelsQuery.data,
				)
			: undefined;
	const modelCount =
		chatModelConfigsQuery.isSuccess && chatModelsQuery.isSuccess
			? modelOptions.length
			: undefined;
	const unsupportedProviderNames = getUnsupportedProviderNames(
		chatModelsQuery.data,
	);

	// Subscribe to live workspace updates so that agent status changes
	// (e.g. connected/disconnected) are reflected without a page refresh.
	const applyWatchedWorkspaceUpdate = useEffectEvent(
		(watchedWorkspaceId: string, next: TypesGen.Workspace) => {
			queryClient.setQueryData<TypesGen.Workspace | undefined>(
				workspaceByIdKey(watchedWorkspaceId),
				(prev) => {
					// Return the same reference when nothing the UI
					// reads has changed. This prevents react-query
					// from notifying subscribers and avoids a full
					// AgentChatPage re-render on every heartbeat.
					const prevAgent = getWorkspaceAgent(prev, chatAgentId);
					const nextAgent = getWorkspaceAgent(next, chatAgentId);
					if (
						prev &&
						prev.latest_build.status === next.latest_build.status &&
						prev.health.healthy === next.health.healthy &&
						prev.name === next.name &&
						prev.owner_name === next.owner_name &&
						prevAgent?.id === nextAgent?.id &&
						prevAgent?.status === nextAgent?.status &&
						prevAgent?.name === nextAgent?.name &&
						prevAgent?.expanded_directory === nextAgent?.expanded_directory &&
						prevAgent?.lifecycle_state === nextAgent?.lifecycle_state
					) {
						return prev;
					}
					return next;
				},
			);
		},
	);
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
						applyWatchedWorkspaceUpdate(
							workspaceId,
							event.parsedMessage.data as TypesGen.Workspace,
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
	const workspaceAgent = getWorkspaceAgent(workspace, chatAgentId);
	const { proxy } = useProxy();

	const chatRecord = chatQuery.data;
	const isArchived = chatRecord?.archived ?? false;
	const isSharedChat = chatRecord?.shared ?? false;
	const isViewerNotOwner =
		chatRecord !== undefined && currentUser.id !== chatRecord.owner_id;
	const isRootChat =
		chatRecord !== undefined && getParentChatID(chatRecord) === undefined;
	const shouldCheckCanShareChat = isRootChat;
	const chatAuthorizationObject =
		chatRecord !== undefined
			? {
					resource_type: "chat" as const,
					owner_id: chatRecord.owner_id,
					organization_id: chatRecord.organization_id,
				}
			: undefined;
	const chatAuthorizationChecks: TypesGen.AuthorizationRequest["checks"] = {};
	if (chatAuthorizationObject !== undefined && shouldCheckCanShareChat) {
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
	const chatOwner = isViewerNotOwner
		? {
				...(chatRecord?.owner_username
					? { username: chatRecord.owner_username }
					: {}),
				...(chatRecord?.owner_name ? { name: chatRecord.owner_name } : {}),
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

	const aiGatewayDisabled = !useAIGatewayEnabled();
	const { store, clearStreamError, upsertCacheMessages } = useChatStore({
		chatID: agentId,
		chatMessages: chatMessagesList,
		chatRecord,
		chatMessagesData,
		chatQueuedMessages,
		setChatErrorReason,
		clearChatErrorReason,
		aiGatewayDisabled,
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

	const effectiveModelOption = modelOptions.find(
		(option) => option.id === effectiveSelectedModel,
	);
	const effectiveReasoningEffort = effectiveModelOption
		? pickReasoningEffort(
				selectedReasoningEffort || chatRecord?.last_reasoning_effort,
				effectiveModelOption.reasoningEfforts ?? [],
				effectiveModelOption.reasoningEffortDefault,
			)
		: undefined;

	const compressionThreshold = resolveCompactionThreshold(
		chatLastModelConfigID,
		userThresholdsQuery.data?.thresholds,
		modelConfigs,
	);
	const hasModelOptions = modelOptions.length > 0;
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
			isChatUsageLimitExceededResponse(error.response.data)
		) {
			const reason: ChatDetailError = {
				kind: "usage_limit",
				message: formatUsageLimitMessage(error.response.data),
			};
			store.setStreamError(reason);
			setChatErrorReason(agentId, reason);
		} else if (isApiError(error)) {
			const detail = error.response?.data?.detail?.trim() || undefined;
			const reason: ChatDetailError = {
				kind: "generic",
				message: getErrorMessage(error, "An unexpected error occurred."),
				...(detail ? { detail } : {}),
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

	const handlePromoteQueuedMessage = (id: number) =>
		runPromoteQueuedMessage({
			id,
			store,
			promoteQueuedMessage,
			agentId,
			clearChatErrorReason,
			handleUsageLimitError,
		});

	const editing = useConversationEditingState({
		chatID: agentId,
		onSend: handleSend,
		onDeleteQueuedMessage: handleDeleteQueuedMessage,
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

	const handlePinAgentAction = () => {
		if (!agentId || isArchived) {
			return;
		}
		requestPinAgent(agentId);
	};

	const handleUnpinAgentAction = () => {
		if (!agentId || isArchived) {
			return;
		}
		requestUnpinAgent(agentId);
	};

	const handleOpenRenameDialogAction =
		onOpenRenameDialog && chatRecord
			? () => {
					if (isArchived) {
						return;
					}
					onOpenRenameDialog(chatRecord);
				}
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
			const originalEditedMessage = chatMessagesList?.find(
				(existingMessage) => existingMessage.id === editedMessageID,
			);
			const originalModelConfigID = originalEditedMessage?.model_config_id;
			const pickerModelConfigID = effectiveSelectedModel || undefined;
			const originalIsSelectable =
				originalModelConfigID !== undefined &&
				modelOptions.some((opt) => opt.id === originalModelConfigID);
			// Only override the original model when the user has switched to
			// a different selectable option. If the original is no longer
			// selectable, the picker is showing a fallback we should not
			// silently use; let the backend preserve the original.
			const editSelectedModelConfigID =
				pickerModelConfigID &&
				originalIsSelectable &&
				pickerModelConfigID !== originalModelConfigID
					? pickerModelConfigID
					: undefined;
			// Omit so the backend preserves the original effort.
			const request: TypesGen.EditChatMessageRequest = {
				content,
				model_config_id: editSelectedModelConfigID,
				reasoning_effort: isEditReasoningEffortDirtyRef.current
					? effectiveReasoningEffort
					: undefined,
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
			if (editSelectedModelConfigID) {
				localStorage.setItem(
					lastModelConfigIDStorageKey,
					editSelectedModelConfigID,
				);
			}
			return;
		}

		const selectedModelConfigID = effectiveSelectedModel || undefined;
		const request: CreateChatMessageRequestWithClearablePlanMode = {
			content,
			model_config_id: selectedModelConfigID,
			reasoning_effort: effectiveReasoningEffort,
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
			// Thinking indicator appears immediately.
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
				sendShortcut={getAgentChatSendShortcut(
					preferencesQuery.data?.agent_chat_send_shortcut,
					preferencesQuery.isLoading,
				)}
				titleElement={titleElement}
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
			key={agentId}
			agentId={agentId}
			sendShortcut={getAgentChatSendShortcut(
				preferencesQuery.data?.agent_chat_send_shortcut,
				preferencesQuery.isLoading,
			)}
			organizationId={chatQuery.data?.organization_id}
			chatTitle={chatTitle}
			parentChat={parentChat}
			persistedError={persistedError}
			isArchived={isArchived}
			isSharedChat={isSharedChat}
			chatOwner={chatOwner}
			canShareChat={canShareChat}
			workspace={workspace}
			workspaceAgent={workspaceAgent}
			chatBuildId={chatQuery.data?.build_id}
			store={store}
			editing={{ ...editing, handleEditUserMessage }}
			effectiveSelectedModel={effectiveSelectedModel}
			setSelectedModel={setSelectedModel}
			modelOptions={modelOptions}
			modelSelectorPlaceholder={modelSelectorPlaceholder}
			modelSelectorHelp={modelSelectorHelp}
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
			isModelCatalogLoading={isModelCatalogLoading}
			planModeEnabled={planModeEnabled}
			onPlanModeToggle={handlePlanModeToggle}
			compressionThreshold={compressionThreshold}
			isInputDisabled={isInputDisabled}
			isSubmissionPending={isSubmissionPending}
			isInterruptPending={isInterruptPending}
			workspaceOptions={workspaceOptions}
			selectedWorkspaceId={selectedWorkspaceId}
			onWorkspaceChange={
				canUpdateChatWorkspace ? handleWorkspaceChange : undefined
			}
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
			handlePinAgentAction={handlePinAgentAction}
			handleUnpinAgentAction={handleUnpinAgentAction}
			handleOpenRenameDialogAction={handleOpenRenameDialogAction}
			isArchivingThisChat={
				isArchiving &&
				(archivingChatId === undefined || archivingChatId === agentId)
			}
			isPinned={(chatRecord?.pin_order ?? 0) > 0}
			isChildChat={parentChatID !== undefined}
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
			chatContext={chatQuery.data?.context}
			workspaceSkills={workspaceSkillsFromChat(chatQuery.data)}
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
