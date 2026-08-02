import { useInfiniteQuery, useQuery } from "react-query";
import {
	chat as chatDetailQuery,
	chatMessagesForInfiniteScroll,
	selectDurableMessageCount,
	selectDurableMessages,
	selectLatestDurableMessageRole,
} from "#/api/queries/chats";
import type * as TypesGen from "#/api/typesGenerated";
import {
	type ChatStore,
	selectHasStreamOverlay,
	selectQueuedMessages,
	useChatSelector,
} from "./chatStore";

// Read facade for durable chat state (messages, queued messages, chat
// status). Render consumers read durable state through these hooks so the
// source can move from the store to the query cache without touching call
// sites. Chat status and messages resolve to the query cache, which is
// canonical for them; the queue still resolves to the store.
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
): readonly TypesGen.ChatQueuedMessage[] =>
	useChatSelector(args.store, selectQueuedMessages);

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
