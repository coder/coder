import { useQuery } from "react-query";
import { chat as chatDetailQuery } from "#/api/queries/chats";
import type * as TypesGen from "#/api/typesGenerated";
import {
	type ChatStore,
	type ChatStoreState,
	selectIsPendingAssistantResponse,
	selectMessagesByID,
	selectOrderedMessageIDs,
	selectQueuedMessages,
	useChatSelector,
} from "./chatStore";

// Read facade for durable chat state (messages, queued messages, chat
// status). Render consumers read durable state through these hooks so the
// source can move from the store to the query cache without touching call
// sites. Chat status resolves to the query cache, which is canonical for it;
// messages and the queue still resolve to the store.
//
// Composition rule: `useChatSelector` feeds its selector result straight to
// `useSyncExternalStore`, which compares snapshots with Object.is on every
// render. A selector that allocates a fresh object or array loops forever.
// Read raw slices through `useChatSelector` and derive in the hook body,
// never inside the selector. React Compiler covers this directory and
// memoizes the derivation against the slices it reads.
type DurableChatArgs = {
	store: ChatStore;
	// Required, and explicitly undefined-able: the query key for the durable
	// reads. Without a chat there is nothing to read.
	chatId: string | undefined;
};

// Primitive selector so count consumers subscribe to the size only, not to
// messagesByID identity.
const selectMessageCount = (state: ChatStoreState): number =>
	state.messagesByID.size;

const isChatMessage = (
	message: TypesGen.ChatMessage | undefined,
): message is TypesGen.ChatMessage => Boolean(message);

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

// Composed from two narrow primitives rather than one store selector: the
// status now lives in the query cache, and widening the store subscription to
// the message list or the stream state would re-render on every chunk.
export const useIsAwaitingFirstStreamChunk = (
	args: DurableChatArgs,
): boolean => {
	const isPendingAssistantResponse = useChatSelector(
		args.store,
		selectIsPendingAssistantResponse,
	);
	const chatStatus = useDurableChatStatus(args);
	return isPendingAssistantResponse && chatStatus === "running";
};
