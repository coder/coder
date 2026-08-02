import type { InfiniteData, QueryClient } from "react-query";
import { useInfiniteQuery, useQuery } from "react-query";
import {
	chat as chatDetailQuery,
	chatKeys,
	chatMessagesForInfiniteScroll,
	selectDurableMessageCount,
	selectDurableMessages,
	selectLatestDurableMessageRole,
} from "#/api/queries/chats";
import type * as TypesGen from "#/api/typesGenerated";
import {
	type ChatStore,
	selectHasStreamOverlay,
	selectSuppressedQueuedMessageIDs,
	useChatSelector,
} from "./chatStore";

// Read facade for durable chat state (messages, queued messages, chat
// status). Render consumers read durable state through these hooks so the
// source can move from the store to the query cache without touching call
// sites. Chat status, durable messages, and the queued messages all resolve
// to the query cache, which is canonical for them.
//
// Composition rule: `useChatSelector` feeds its selector result straight to
// `useSyncExternalStore`, which compares snapshots with Object.is on every
// render. A selector that allocates a fresh object or array loops forever.
// Read raw slices through `useChatSelector` and derive in the hook body,
// never inside the selector. React Compiler covers this directory and
// memoizes the derivation against the slices it reads.
//
// Query-observer rule: every durable-message read mounts its own observer on
// `chatKeys.messages` with the SAME base options plus a module-level `select`,
// so react-query can memoize the select per observer and structural sharing
// keeps the result reference stable. The hooks MUST use `useInfiniteQuery`: a
// plain `useQuery` observer on that key would fetch without
// `infiniteQueryBehavior` and install a bare page in place of the pages.
// Pagination stays with AgentChatPage; a facade observer never calls
// `fetchNextPage` or `refetch`.
type DurableChatArgs = {
	store: ChatStore;
	// Required, and explicitly undefined-able: the query key for the durable
	// reads. Without a chat there is nothing to read.
	chatId: string | undefined;
};

// Module-level so the query result identity is stable across renders.
const selectChatStatusFromDetail = (
	chat: TypesGen.Chat | undefined,
): TypesGen.ChatStatus | null => chat?.status ?? null;

export const useDurableChatStatus = (
	args: DurableChatArgs,
): TypesGen.ChatStatus | null =>
	useQuery({
		...chatDetailQuery(args.chatId ?? ""),
		enabled: Boolean(args.chatId),
		// Selecting the status keeps consumers off every other detail field.
		select: selectChatStatusFromDetail,
	}).data ?? null;

const EMPTY_MESSAGES: readonly TypesGen.ChatMessage[] = [];
const EMPTY_QUEUED_MESSAGES: readonly TypesGen.ChatQueuedMessage[] = [];

/**
 * The queue travels with the messages response, so page 0 of the paginated
 * messages cache is where it lives. Module level so react-query memoizes it
 * per observer and structural sharing keeps the result reference stable.
 */
export const selectCanonicalQueuedMessages = (
	data: InfiniteData<TypesGen.ChatMessagesResponse>,
): readonly TypesGen.ChatQueuedMessage[] =>
	data.pages[0]?.queued_messages ?? EMPTY_QUEUED_MESSAGES;

/**
 * The RAW queue as the server last reported it, including rows an optimistic
 * removal has suppressed. Imperative readers that have to reason about the
 * SERVER's queue use this: the server promotes its head by `position`, which
 * includes a row the client is already hiding.
 */
export const readCanonicalQueuedMessages = (
	queryClient: QueryClient,
	chatId: string | undefined,
): readonly TypesGen.ChatQueuedMessage[] | undefined => {
	if (!chatId) {
		return undefined;
	}
	return queryClient.getQueryData<
		InfiniteData<TypesGen.ChatMessagesResponse> | undefined
	>(chatKeys.messages(chatId))?.pages[0]?.queued_messages;
};

/**
 * The canonical queue minus the suppression markers: what the user is allowed
 * to see. Optimistic removals never write the cache, so this subtraction is
 * the only thing that hides them, and a failed mutation only has to drop the
 * marker.
 */
export const selectEffectiveQueuedMessages = (
	canonicalQueuedMessages: readonly TypesGen.ChatQueuedMessage[],
	suppressedQueuedMessageIDs: ReadonlySet<number>,
): readonly TypesGen.ChatQueuedMessage[] =>
	suppressedQueuedMessageIDs.size === 0
		? canonicalQueuedMessages
		: canonicalQueuedMessages.filter(
				(message) => !suppressedQueuedMessageIDs.has(message.id),
			);

/**
 * Imperative counterpart of the render facade, for callers that hold a query
 * client rather than a hook result.
 */
export const readEffectiveQueuedMessages = (
	queryClient: QueryClient,
	store: ChatStore,
	chatId: string | undefined,
): readonly TypesGen.ChatQueuedMessage[] =>
	selectEffectiveQueuedMessages(
		readCanonicalQueuedMessages(queryClient, chatId) ?? EMPTY_QUEUED_MESSAGES,
		store.getSnapshot().suppressedQueuedMessageIDs,
	);

// Flat, ascending message list read from the canonical paginated cache.
export const useDurableMessageList = (
	args: DurableChatArgs,
): readonly TypesGen.ChatMessage[] =>
	useInfiniteQuery({
		...chatMessagesForInfiniteScroll(args.chatId ?? ""),
		enabled: Boolean(args.chatId),
		select: selectDurableMessages,
	}).data ?? EMPTY_MESSAGES;

export const useDurableMessageCount = (args: DurableChatArgs): number =>
	useInfiniteQuery({
		...chatMessagesForInfiniteScroll(args.chatId ?? ""),
		enabled: Boolean(args.chatId),
		select: selectDurableMessageCount,
	}).data ?? 0;

/**
 * Decides whether the finalizing overlay snapshot must be dropped, for the
 * durable-reading parent that holds both inputs in one render. Exact ID
 * membership, never a `>=` comparison: a newer durable message must not
 * suppress an overlay whose own finalized message has not landed in the cache.
 */
export const shouldSuppressFinalizedOverlay = (
	finalizingMessageID: number | null,
	messages: readonly TypesGen.ChatMessage[],
): boolean =>
	finalizingMessageID !== null &&
	messages.some((message) => message.id === finalizingMessageID);

export const useDurableQueuedMessages = (
	args: DurableChatArgs,
): readonly TypesGen.ChatQueuedMessage[] => {
	const canonicalQueuedMessages =
		useInfiniteQuery({
			...chatMessagesForInfiniteScroll(args.chatId ?? ""),
			enabled: Boolean(args.chatId),
			select: selectCanonicalQueuedMessages,
		}).data ?? EMPTY_QUEUED_MESSAGES;
	const suppressedQueuedMessageIDs = useChatSelector(
		args.store,
		selectSuppressedQueuedMessageIDs,
	);
	// Combined in the hook body, never inside either selector: the store
	// selector has to stay a raw slice and the query select has to stay module
	// level, so this is the only place the two projections can meet.
	return selectEffectiveQueuedMessages(
		canonicalQueuedMessages,
		suppressedQueuedMessageIDs,
	);
};

// Composed from two narrow primitives rather than one selector over the whole
// conversation: the role of the newest durable message is a primitive, and the
// overlay flag keeps the indicator off screen while a tail is still rendering,
// including the finalization handoff window.
export const useIsAwaitingFirstStreamChunk = (
	args: DurableChatArgs,
): boolean => {
	const hasStreamOverlay = useChatSelector(args.store, selectHasStreamOverlay);
	const latestDurableRole = useInfiniteQuery({
		...chatMessagesForInfiniteScroll(args.chatId ?? ""),
		enabled: Boolean(args.chatId),
		select: selectLatestDurableMessageRole,
	}).data;
	const chatStatus = useDurableChatStatus(args);
	return (
		!hasStreamOverlay &&
		latestDurableRole !== "assistant" &&
		chatStatus === "running"
	);
};
