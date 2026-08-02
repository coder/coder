import {
	type FC,
	useEffect,
	useEffectEvent,
	useLayoutEffect,
	useRef,
	useState,
} from "react";

import type { QueryClient } from "react-query";
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
	chatKeys,
	chatMessagesForInfiniteScroll,
	chatModelConfigs,
	chatModels,
	chatProviderConfigs,
	compactChat,
	createChatMessage,
	deleteChatQueuedMessage,
	editChatMessage,
	interruptChat,
	mcpServerConfigs,
	patchChatEverywhere,
	promoteChatQueuedMessage,
	readCachedChatSnapshotVersion,
	rollbackOptimisticChatStatus,
	selectDurableMessages,
	updateChatPlanMode,
	updateChatWorkspace,
	userChatDebugLogging,
	userChatProviderConfigs,
	userCompactionThresholds,
	writeOptimisticChatStatus,
} from "#/api/queries/chats";
import { deploymentSSHConfig } from "#/api/queries/deployment";
import { userSkills } from "#/api/queries/userSkills";
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
import type { AgentsPageOutletContext } from "./AgentsPageLayout";
import type { ChatMessageInputRef } from "./components/AgentChatInput";
import { normalizeChatErrorPayload } from "./components/ChatConversation/chatError";
import {
	getParentChatID,
	getWorkspaceAgent,
} from "./components/ChatConversation/chatHelpers";
import { runExclusiveQueueMutation } from "./components/ChatConversation/chatQueueMutations";
import {
	type ChatStore,
	ChatStoreContext,
	type ChatStoreState,
} from "./components/ChatConversation/chatStore";
import {
	readEffectiveQueuedMessages,
	useDurableChatStatus,
} from "./components/ChatConversation/durableChat";
import { useChatStore } from "./components/ChatConversation/useChatStore";
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
	COMPACT_SLASH_COMMAND,
	chatSlashCommandTriggerText,
	resolveChatSlashCommandAvailability,
} from "./utils/slashCommands";
import {
	type ChatDetailError,
	formatUsageLimitMessage,
	isChatHookDeniedResponse,
	isChatHookDispatchFailedResponse,
	isChatUsageLimitExceededResponse,
} from "./utils/usageLimitMessage";

/** localStorage key controlling whether the right panel is visible. */
export const RIGHT_PANEL_OPEN_KEY = "agents.right-panel-open";

const lastModelConfigIDStorageKey = "agents.last-model-config-id";
class CompactCommandPendingError extends Error {}

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

/**
 * Restores the transient store slices an optimistic request overwrote. Chat
 * status is not among them: it lives in the query cache, and its rollback is
 * token-scoped through `rollbackOptimisticChatStatus`. Neither is the queue:
 * the cache is canonical for it, and an optimistic queue removal is a
 * read-time marker the failing caller unsuppresses on its own.
 *
 * @internal Exported for testing.
 */
export const restoreOptimisticRequestSnapshot = (
	store: Pick<ChatStore, "batch" | "setStreamError" | "setStreamState">,
	snapshot: Pick<ChatStoreState, "streamError" | "streamState">,
): void => {
	store.batch(() => {
		store.setStreamState(snapshot.streamState);
		store.setStreamError(snapshot.streamError);
	});
};

/**
 * Runs the optimistic queued-message promotion flow.
 *
 * The promote endpoint returns 202 Accepted with no message body, so the
 * actual user message is delivered via SSE or the messages REST endpoint.
 * Suppressing the promoted ID is the whole optimistic update: it hides the row
 * instantly, and it keeps hiding it through the transient reordered queue the
 * running-case backend publishes before auto-promoting it. The cache is never
 * written, so a failure only has to drop the marker, and a queue snapshot that
 * lands mid-request is not clobbered by a rollback.
 *
 * @internal Exported for testing.
 */
export const runPromoteQueuedMessage = async (params: {
	id: number;
	chatID: string | undefined;
	store: Pick<
		ChatStore,
		| "batch"
		| "clearStreamError"
		| "clearStreamState"
		| "getSnapshot"
		| "setStreamError"
		| "setStreamState"
		| "suppressQueuedMessageIDs"
		| "unsuppressQueuedMessageIDs"
	>;
	queryClient: QueryClient;
	promoteQueuedMessage: (id: number) => Promise<void>;
	agentId: string | undefined;
	clearChatErrorReason: (chatID: string) => void;
	handleUsageLimitError: (error: unknown) => void;
}): Promise<void> => {
	const {
		id,
		chatID,
		store,
		queryClient,
		promoteQueuedMessage,
		agentId,
		clearChatErrorReason,
		handleUsageLimitError,
	} = params;
	const run = async (): Promise<void> => {
		const previousSnapshot = store.getSnapshot();
		store.batch(() => {
			store.suppressQueuedMessageIDs([id]);
			store.clearStreamState();
			store.clearStreamError();
		});
		const optimisticStatusToken = agentId
			? writeOptimisticChatStatus(queryClient, agentId, "running")
			: undefined;
		if (agentId) {
			clearChatErrorReason(agentId);
		}
		try {
			await promoteQueuedMessage(id);
		} catch (error) {
			store.unsuppressQueuedMessageIDs([id]);
			if (agentId && optimisticStatusToken !== undefined) {
				rollbackOptimisticChatStatus(
					queryClient,
					agentId,
					optimisticStatusToken,
				);
			}
			restoreOptimisticRequestSnapshot(store, previousSnapshot);
			handleUsageLimitError(error);
			throw error;
		}
	};
	// Markers and request together: a marker placed before the lock belongs to
	// an operation that has not started, so its failure path could lift a marker
	// another operation took ownership of meanwhile.
	await (chatID ? runExclusiveQueueMutation(chatID, run) : run());
};

/**
 * Runs the optimistic queued-message deletion flow.
 *
 * The removal is a read-time suppression marker, not a cache write: the cache
 * stays server-truthful, the row disappears from the visible queue instantly
 * even while a pagination fetch is buffering cache writes, and a failure only
 * unsuppresses. The server's own `queue_update` retires the marker by omitting
 * the id.
 *
 * @internal Exported for testing.
 */
export const runDeleteQueuedMessage = async (params: {
	id: number;
	chatID: string | undefined;
	store: Pick<
		ChatStore,
		"suppressQueuedMessageIDs" | "unsuppressQueuedMessageIDs"
	>;
	deleteQueuedMessage: (id: number) => Promise<void>;
}): Promise<void> => {
	const { id, chatID, store, deleteQueuedMessage } = params;
	const run = async (): Promise<void> => {
		store.suppressQueuedMessageIDs([id]);
		try {
			await deleteQueuedMessage(id);
		} catch (error) {
			// Lifts this operation's ordinary suppression only. If a send promoted
			// the same ID meanwhile, that promotion veto is not ours to retire.
			store.unsuppressQueuedMessageIDs([id]);
			throw error;
		}
	};
	await (chatID ? runExclusiveQueueMutation(chatID, run) : run());
};

/**
 * Asserts the turn a send just started, unless the server already reported a
 * status while the request was in flight.
 *
 * The cached snapshot version is the fence: optimistic writes never advance
 * it, so a change means a server status landed during the request, and that
 * status is newer than anything this path can claim.
 *
 * @internal Exported for testing.
 */
export const assertTurnStartedAfterSend = (params: {
	store: Pick<ChatStore, "clearStreamState">;
	queryClient: QueryClient;
	chatId: string;
	snapshotVersionBeforeSend: number;
}): void => {
	const { store, queryClient, chatId, snapshotVersionBeforeSend } = params;
	if (
		readCachedChatSnapshotVersion(queryClient, chatId) !==
		snapshotVersionBeforeSend
	) {
		return;
	}
	store.clearStreamState();
	// The server accepted the message, so it will start processing. A status
	// event carries a newer snapshot version and supersedes this write,
	// whichever status it reports.
	writeOptimisticChatStatus(queryClient, chatId, "running");
};

const buildPromotedQueueReconciliation = (
	queuedMessages: readonly TypesGen.ChatQueuedMessage[],
	promotedQueuedMessageID: number | undefined,
	queuedTail: TypesGen.ChatQueuedMessage | undefined,
	hasObservedQueuedMessageID: (id: number) => boolean,
): readonly TypesGen.ChatQueuedMessage[] | undefined => {
	// Queue IDs are a positive bigserial, so an absent or zero field means the
	// send promoted nothing.
	if (promotedQueuedMessageID === undefined || promotedQueuedMessageID === 0) {
		return undefined;
	}
	const remaining = queuedMessages.filter(
		(message) => message.id !== promotedQueuedMessageID,
	);
	const tailPending =
		queuedTail !== undefined &&
		// Defensive: the state machine inserts the tail after popping the head,
		// so they are always distinct rows. Appending a tail that names the
		// promoted ID would re-add the row this projection just removed.
		queuedTail.id !== promotedQueuedMessageID &&
		!remaining.some((message) => message.id === queuedTail.id) &&
		!hasObservedQueuedMessageID(queuedTail.id);
	return tailPending ? [...remaining, queuedTail] : remaining;
};

// The inactive-chat variant: no socket is watching, so nothing local can have
// observed the tail yet, and the cached queue is the only baseline. Without a
// cached queue there is no page 0 to amend, and re-entry refetches it.
export const buildInactiveChatQueueReconciliation = (
	cachedQueuedMessages: readonly TypesGen.ChatQueuedMessage[] | undefined,
	promotedQueuedMessageID: number | undefined,
	queuedTail: TypesGen.ChatQueuedMessage | undefined,
): readonly TypesGen.ChatQueuedMessage[] | undefined =>
	cachedQueuedMessages === undefined
		? undefined
		: buildPromotedQueueReconciliation(
				cachedQueuedMessages,
				promotedQueuedMessageID,
				queuedTail,
				() => false,
			);

/**
 * Projects the queue a promoting send left behind: the canonical queue minus
 * the row the server reported promoting, plus the tail the response returned.
 * Both facts are server-confirmed, so the result is a server-DERIVED value the
 * caller may write to the canonical cache. It is the one cache write the
 * client originates, because a marker can only subtract and this one adds.
 *
 * The promoted ID comes from `promoted_queued_message_id` on the send
 * response, so it is exact even when the server promoted a row this client
 * does not order first. `response.messages` never participates: the promoted
 * row is not necessarily `messages[0]`, and message content must not decide
 * queue identity. `promotedQueuedMessageIDs` still fences snapshots derived
 * before the promotion committed, which the exact ID makes provable.
 */
export const reconcilePromotedQueueHead = (
	store: Pick<
		ChatStore,
		"markQueuedMessagePromoted" | "hasObservedQueuedMessageID"
	>,
	canonicalQueuedMessages: readonly TypesGen.ChatQueuedMessage[],
	promotedQueuedMessageID: number | undefined,
	queuedTail: TypesGen.ChatQueuedMessage | undefined,
): readonly TypesGen.ChatQueuedMessage[] | undefined => {
	const next = buildPromotedQueueReconciliation(
		canonicalQueuedMessages,
		promotedQueuedMessageID,
		queuedTail,
		store.hasObservedQueuedMessageID,
	);
	if (!next || promotedQueuedMessageID === undefined) {
		return next;
	}
	// The server reported this row as promoted, so its queue row is gone.
	store.markQueuedMessagePromoted(promotedQueuedMessageID);
	return next;
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

/**
 * Runs the optimistic queue-clearing edit flow.
 *
 * An edit restarts the turn and clears the server queue, so the visible queue
 * is hidden the same way a delete hides one row: read-time markers, never a
 * cache write. A cache clear would need a rollback, and that rollback would
 * clobber a `queue_update` that landed while the edit was in flight.
 *
 * Everything the operation owns runs INSIDE the queue lock, the marker
 * capture included. Captured before FIFO ownership, the marker list would
 * describe a queue another operation is still changing, and this failure path
 * would then lift markers that operation owns.
 *
 * @internal Exported for testing.
 */
export const runQueueSuppressingEdit = (params: {
	chatID: string;
	store: ChatStore;
	queryClient: QueryClient;
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
	clearChatErrorReason: (chatID: string) => void;
	clearStreamError: () => void;
	handleUsageLimitError: (error: unknown) => void;
}): Promise<void> => {
	const {
		chatID,
		store,
		queryClient,
		editMessage,
		editArgs,
		scrollToBottom,
		clearChatErrorReason,
		clearStreamError,
		handleUsageLimitError,
	} = params;
	return runExclusiveQueueMutation(chatID, async () => {
		const previousSnapshot = store.getSnapshot();
		const suppressedForEdit = readEffectiveQueuedMessages(
			queryClient,
			store,
			chatID,
		).map((message) => message.id);
		clearChatErrorReason(chatID);
		clearStreamError();
		store.batch(() => {
			store.suppressQueuedMessageIDs(suppressedForEdit);
			store.clearStreamState();
		});
		const optimisticStatusToken = writeOptimisticChatStatus(
			queryClient,
			chatID,
			"running",
		);
		await submitEditAndScroll({
			editMessage,
			editArgs,
			scrollToBottom,
			onError: (error) => {
				// Lifts this edit's ordinary suppression only; a send's promotion
				// veto on the same ID is retired by the server snapshot that omits
				// the promoted row.
				store.unsuppressQueuedMessageIDs(suppressedForEdit);
				rollbackOptimisticChatStatus(
					queryClient,
					chatID,
					optimisticStatusToken,
				);
				restoreOptimisticRequestSnapshot(store, previousSnapshot);
				handleUsageLimitError(error);
				// Hook dispatch failures can park an idle chat in error before
				// returning the request error. The refetched detail carries a
				// snapshot version, so the comparator decides whether it is newer
				// than any status the socket delivered meanwhile.
				void queryClient.invalidateQueries({
					queryKey: chatKeys.detail(chatID),
					exact: true,
				});
			},
		});
	});
};

/**
 * Runs a chat turn behind a synchronous in-flight guard.
 *
 * A submission occupies its chat's queue-position slot from the moment it is
 * accepted, which is BEFORE its mutation reports pending: the request only
 * starts once the queue lock is free. A second submission entering that
 * window would send the same turn twice. The ref makes the guard synchronous,
 * because a state update is invisible to a handler that already captured its
 * closure; the setter mirrors it so the composer can disable.
 *
 * @internal Exported for testing.
 */
export const runGuardedChatTurn = async (params: {
	inFlightRef: { current: boolean };
	setInFlight: (inFlight: boolean) => void;
	run: () => Promise<void>;
}): Promise<void> => {
	const { inFlightRef, setInFlight, run } = params;
	if (inFlightRef.current) {
		return;
	}
	inFlightRef.current = true;
	setInFlight(true);
	try {
		await run();
	} finally {
		inFlightRef.current = false;
		setInFlight(false);
	}
};

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
			if (error instanceof CompactCommandPendingError) {
				return;
			}
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
		// history or queued message, the original draft (possibly
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
					// QuotaExceededError, silently discard the draft.
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
	} = useOutletContext<AgentsPageOutletContext>();
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

	// Flatten paginated messages into chronological order with the same
	// module-level select the durable read facade uses, so the page and the
	// facade cannot disagree about the conversation.
	const chatMessagesList = chatMessagesQuery.data
		? selectDurableMessages(chatMessagesQuery.data)
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
	const { isPending: isCompactPending, mutateAsync: compact } = useMutation(
		compactChat(queryClient, agentId ?? ""),
	);
	// A skill named "compact" takes precedence over built-in /compact. Until
	// both skill sources resolve, exact submissions wait to avoid shadowing it.
	const personalSkillsQuery = useQuery({
		...userSkills(),
		staleTime: 60_000,
	});
	const chatWorkspaceSkills = workspaceSkillsFromChat(chatQuery.data);
	const compactCommandResolution = resolveChatSlashCommandAvailability(
		COMPACT_SLASH_COMMAND,
		personalSkillsQuery.isSuccess ? personalSkillsQuery.data : undefined,
		chatWorkspaceSkills,
	);
	const { mutateAsync: deleteQueuedMessage } = useMutation(
		deleteChatQueuedMessage(queryClient, agentId ?? ""),
	);
	const { mutateAsync: promoteQueuedMessage } = useMutation(
		promoteChatQueuedMessage(queryClient, agentId ?? ""),
	);
	// A submission holds a queue-position slot from the moment it is
	// accepted, which is BEFORE its mutation reports pending. See
	// `runGuardedChatTurn`.
	const chatTurnInFlightRef = useRef(false);
	const [isChatTurnInFlight, setIsChatTurnInFlight] = useState(false);
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
		patchChatEverywhere(queryClient, chatId, (chat) =>
			chat.plan_mode === planMode ? chat : { ...chat, plan_mode: planMode },
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
	const {
		store,
		clearStreamError,
		writeCanonicalQueuedMessages,
		readCanonicalQueuedMessages,
		upsertCacheMessages,
	} = useChatStore({
		chatID: agentId,
		chatMessages: chatMessagesList,
		chatRecord,
		setChatErrorReason,
		clearChatErrorReason,
		aiGatewayDisabled,
	});
	const liveChatStatus = useDurableChatStatus({ store, chatId: agentId });
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
		isSendPending ||
		isEditPending ||
		isInterruptPending ||
		isCompactPending ||
		isChatTurnInFlight;
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

	const handleDeleteQueuedMessage = (id: number) =>
		runDeleteQueuedMessage({
			id,
			chatID: agentId,
			store,
			deleteQueuedMessage,
		});

	const handlePromoteQueuedMessage = (id: number) =>
		runPromoteQueuedMessage({
			id,
			chatID: agentId,
			store,
			queryClient,
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

	// The query cache is canonical for durable messages, so a successful
	// messages query means the DOM already carries them and the parent can
	// scroll. There is no second source left to wait for.
	const chatReadyFiredRef = useRef<string | null>(null);
	useEffect(() => {
		if (chatReadyFiredRef.current === agentId || !chatMessagesQuery.isSuccess) {
			return;
		}
		chatReadyFiredRef.current = agentId ?? null;
		onChatReady();
	}, [onChatReady, chatMessagesQuery.isSuccess, agentId]);

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

	type ChatTurnSubmission = {
		message: string;
		attachments?: readonly PendingAttachment[];
		editedMessageID?: number;
		useComposerContent?: boolean;
		planModeSwitch?: PlanModeSwitch;
	};

	// Guards the whole turn, the queue lock wait included, so a second
	// submission cannot slip past a mutation that has not started reporting
	// pending yet.
	function submitChatTurn(submission: ChatTurnSubmission): Promise<void> {
		return runGuardedChatTurn({
			inFlightRef: chatTurnInFlightRef,
			setInFlight: setIsChatTurnInFlight,
			run: () => runChatTurn(submission),
		});
	}

	async function runChatTurn({
		message,
		attachments,
		editedMessageID,
		useComposerContent = true,
		planModeSwitch,
	}: ChatTurnSubmission) {
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

		// "/compact" on its own (no attachments or file references)
		// requests a manual context compaction instead of sending a
		// message. Only new sends are intercepted; history and queued
		// edits keep their original meaning, and a personal or workspace
		// skill named "compact" takes precedence so the command cannot shadow it.
		const isExactCompactSubmission =
			editedMessageID === undefined &&
			editing.editingQueuedMessageID === null &&
			content.length === 1 &&
			content[0].type === "text" &&
			content[0].text?.trim() ===
				chatSlashCommandTriggerText(COMPACT_SLASH_COMMAND);
		if (isExactCompactSubmission && compactCommandResolution === "pending") {
			toast.info(
				"Checking whether /compact is available. Try again in a moment.",
			);
			throw new CompactCommandPendingError();
		}
		if (isExactCompactSubmission && compactCommandResolution === "available") {
			// Optimistically show the running state before awaiting so
			// a fast compaction cannot race this write: the worker's
			// authoritative waiting status may arrive over the stream
			// before the POST resolves and must not be overwritten.
			const previousSnapshot = store.getSnapshot();
			clearChatErrorReason(agentId);
			clearStreamError();
			store.clearStreamState();
			const optimisticStatusToken = writeOptimisticChatStatus(
				queryClient,
				agentId,
				"running",
			);
			scrollToBottomRef.current?.();
			try {
				await compact();
			} catch (error) {
				rollbackOptimisticChatStatus(
					queryClient,
					agentId,
					optimisticStatusToken,
				);
				restoreOptimisticRequestSnapshot(store, previousSnapshot);
				if (
					isApiError(error) &&
					error.response?.status === 409 &&
					isChatUsageLimitExceededResponse(error.response.data)
				) {
					handleUsageLimitError(error);
				} else {
					toast.error(getErrorMessage(error, "Failed to compact chat."));
				}
				throw error;
			}
			return;
		}

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
			await runQueueSuppressingEdit({
				chatID: agentId,
				store,
				queryClient,
				editMessage,
				editArgs: {
					messageId: editedMessageID,
					optimisticMessage,
					req: request,
				},
				scrollToBottom: scrollToBottomRef.current,
				clearChatErrorReason,
				clearStreamError,
				handleUsageLimitError,
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

		// The reconciliation runs inside the queue-mutation lock so a delete
		// or promote committing between the send and the projection cannot
		// interleave with the queue write this send derives.
		await runExclusiveQueueMutation(agentId, async () => {
			// The cached snapshot version is the send-path fence: optimistic
			// writes never advance it, so it changes only when the server
			// reported a status during the request.
			const snapshotVersionBeforeSend = readCachedChatSnapshotVersion(
				queryClient,
				agentId,
			);

			// Don't clear stream state before the POST completes.
			// For queued sends the WebSocket status events handle
			// clearing; for non-queued sends we clear explicitly
			// below. Clearing eagerly causes a visible cutoff.
			let sendResponse: Awaited<ReturnType<typeof sendMessage>>;
			try {
				sendResponse = await sendMessage(request);
			} catch (error) {
				handleUsageLimitError(error);
				// Hook dispatch failures can park an idle chat in error before
				// returning the request error. The refetched detail carries a
				// snapshot version, so the comparator decides whether it is newer
				// than any status the socket delivered meanwhile.
				void queryClient.invalidateQueries({
					queryKey: chatKeys.detail(agentId),
					exact: true,
				});
				throw error;
			}
			const isActiveChat = store.getActiveChatID() === agentId;
			// Waiting for the WebSocket on non-queued sends leaves stale stream state visible.
			if (!sendResponse.queued && isActiveChat) {
				assertTurnStartedAfterSend({
					store,
					queryClient,
					chatId: agentId,
					snapshotVersionBeforeSend,
				});
			}
			// Upsert the full batch because a queued send can insert a promoted head below
			// the highest cached ID, which a reconnect would skip.
			const insertedMessages =
				sendResponse.messages ??
				(sendResponse.message ? [sendResponse.message] : []);
			if (insertedMessages.length > 0) {
				upsertCacheMessages(insertedMessages);
			}
			if (!sendResponse.queued) {
				return;
			}
			// The server names the row it promoted; the message batch never
			// decides queue identity.
			const promotedQueuedMessageID = sendResponse.promoted_queued_message_id;
			const reconciledQueue = isActiveChat
				? reconcilePromotedQueueHead(
						store,
						readCanonicalQueuedMessages() ?? [],
						promotedQueuedMessageID,
						sendResponse.queued_message,
					)
				: buildInactiveChatQueueReconciliation(
						readCanonicalQueuedMessages(),
						promotedQueuedMessageID,
						sendResponse.queued_message,
					);
			if (!reconciledQueue) {
				return;
			}
			writeCanonicalQueuedMessages(reconciledQueue);
			// A promoted head starts a turn, but any server status received during the
			// request is newer and must win.
			if (isActiveChat) {
				assertTurnStartedAfterSend({
					store,
					queryClient,
					chatId: agentId,
					snapshotVersionBeforeSend,
				});
			}
		});
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
		<ChatStoreContext value={store}>
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
				messageCount={chatMessagesList?.length ?? 0}
				desktopChatId={desktopEnabled ? agentId : undefined}
				mcpServers={mcpServers}
				selectedMCPServerIds={effectiveMCPServerIds}
				onMCPSelectionChange={handleMCPSelectionChange}
				onMCPAuthComplete={handleMCPAuthComplete}
				chatContext={chatQuery.data?.context}
				workspaceSkills={workspaceSkillsFromChat(chatQuery.data)}
			/>
		</ChatStoreContext>
	);
};

// Keyed wrapper so that navigating between agents (changing the
// :agentId param) fully remounts the component, resetting all
// internal state (drafts, editing, queries) cleanly.
const KeyedAgentChatPage: FC = () => {
	const { agentId } = useParams<{ agentId: string }>();
	return <AgentChatPage key={agentId} />;
};

export default KeyedAgentChatPage;
