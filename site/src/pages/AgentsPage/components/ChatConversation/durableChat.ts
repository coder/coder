import type * as TypesGen from "#/api/typesGenerated";
import {
	type ChatStore,
	type ChatStoreState,
	selectChatStatus,
	selectIsAwaitingFirstStreamChunk,
	selectMessagesByID,
	selectOrderedMessageIDs,
	selectQueuedMessages,
	useChatSelector,
} from "./chatStore";

// Read facade for durable chat state (messages, queued messages, chat
// status). Render consumers read durable state through these hooks so the
// source can move from the store to the query cache without touching call
// sites. Today every hook resolves to the store, so output is identical to
// reading the selectors directly.
//
// Composition rule: `useChatSelector` feeds its selector result straight to
// `useSyncExternalStore`, which compares snapshots with Object.is on every
// render. A selector that allocates a fresh object or array loops forever.
// Read raw slices through `useChatSelector` and derive in the hook body,
// never inside the selector. React Compiler covers this directory and
// memoizes the derivation against the slices it reads.
type DurableChatArgs = {
	store: ChatStore;
	// Accepted for the query key that will back these reads once durable
	// state moves to the cache. Unused while the store is the source.
	chatId?: string;
};

// Primitive selector so count consumers subscribe to the size only, not to
// messagesByID identity.
const selectMessageCount = (state: ChatStoreState): number =>
	state.messagesByID.size;

const isChatMessage = (
	message: TypesGen.ChatMessage | undefined,
): message is TypesGen.ChatMessage => Boolean(message);

export const useDurableChatStatus = (
	args: DurableChatArgs,
): TypesGen.ChatStatus | null => useChatSelector(args.store, selectChatStatus);

// Flat, ascending message list. messagesByID and orderedMessageIDs stay
// internal to the store; both durable sources agree on the flat shape.
export const useDurableMessageList = (
	args: DurableChatArgs,
): readonly TypesGen.ChatMessage[] => {
	const messagesByID = useChatSelector(args.store, selectMessagesByID);
	const orderedMessageIDs = useChatSelector(
		args.store,
		selectOrderedMessageIDs,
	);

	return orderedMessageIDs
		.map((messageID) => {
			const message = messagesByID.get(messageID);
			if (!message && process.env.NODE_ENV !== "production") {
				console.warn(
					`[durableChat] orderedMessageIDs contains ID ${messageID} ` +
						"not found in messagesByID. This may indicate a store/cache " +
						"desync bug.",
				);
			}
			return message;
		})
		.filter(isChatMessage);
};

export const useDurableMessageCount = (args: DurableChatArgs): number =>
	useChatSelector(args.store, selectMessageCount);

export const useDurableQueuedMessages = (
	args: DurableChatArgs,
): readonly TypesGen.ChatQueuedMessage[] =>
	useChatSelector(args.store, selectQueuedMessages);

// Delegates to the boolean selector, which reads only the latest message,
// the chat status, and whether a stream exists. Composing it from the full
// message list would widen the subscription to any content change.
export const useIsAwaitingFirstStreamChunk = (args: DurableChatArgs): boolean =>
	useChatSelector(args.store, selectIsAwaitingFirstStreamChunk);
