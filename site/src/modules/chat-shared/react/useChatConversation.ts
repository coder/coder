import type * as TypesGen from "api/typesGenerated";
import { useCallback, useEffect, useRef, useState } from "react";
import { useChatRuntimeContext } from "./ChatRuntimeProvider";

/** @public Result state for the shared chat conversation hook. */
export type UseChatConversationResult = {
	chat: TypesGen.Chat | null;
	isLoading: boolean;
	error: unknown;
	refetch: () => Promise<void>;
};

const getLastDurableMessageId = (
	messages: readonly TypesGen.ChatMessage[],
): number | undefined => {
	const lastMessage = messages[messages.length - 1];
	return lastMessage?.id;
};

/** @public Hydrates and tracks a shared chat conversation. */
export const useChatConversation = (
	chatId: string | null | undefined,
): UseChatConversationResult => {
	const {
		runtime,
		store,
		activeChatId,
		setActiveChatId,
		lastDurableMessageId,
		selectionToken,
	} = useChatRuntimeContext();
	const activeChatIdRef = useRef(activeChatId);
	activeChatIdRef.current = activeChatId;
	const [chat, setChat] = useState<TypesGen.Chat | null>(null);
	const [isLoading, setIsLoading] = useState(false);
	const [error, setError] = useState<unknown>(null);

	const resetConversationState = useCallback(() => {
		store.replaceMessages([]);
		store.setQueuedMessages([]);
		store.setChatStatus(null);
		store.resetTransientState();
		lastDurableMessageId.current = undefined;
		setActiveChatId(null);
	}, [lastDurableMessageId, setActiveChatId, store]);

	const loadConversation = useCallback(
		async (requestedChatId: string | null | undefined): Promise<void> => {
			const requestToken = ++selectionToken.current;
			if (!requestedChatId) {
				resetConversationState();
				setChat(null);
				setIsLoading(false);
				setError(null);
				return;
			}

			const isSelectingDifferentChat =
				activeChatIdRef.current !== requestedChatId;
			setActiveChatId(null);
			store.resetTransientState();
			if (isSelectingDifferentChat) {
				store.replaceMessages([]);
				store.setQueuedMessages([]);
				store.setChatStatus(null);
				lastDurableMessageId.current = undefined;
				setChat(null);
			}
			setIsLoading(true);
			setError(null);

			try {
				const detail = await runtime.getChat(requestedChatId);
				if (selectionToken.current !== requestToken) {
					return;
				}
				if (detail.chat.id !== requestedChatId) {
					throw new Error(
						`Expected chat ${requestedChatId} but received ${detail.chat.id}.`,
					);
				}
				store.replaceMessages(detail.messages);
				store.setQueuedMessages(detail.queued_messages);
				store.setChatStatus(detail.chat.status ?? null);
				lastDurableMessageId.current = getLastDurableMessageId(detail.messages);
				setActiveChatId(requestedChatId);
				setChat(detail.chat);
				setIsLoading(false);
			} catch (nextError) {
				if (selectionToken.current !== requestToken) {
					return;
				}
				setError(nextError);
				setIsLoading(false);
			}
		},
		[
			lastDurableMessageId,
			resetConversationState,
			runtime,
			selectionToken,
			setActiveChatId,
			store,
		],
	);

	useEffect(() => {
		void loadConversation(chatId);
	}, [chatId, loadConversation]);

	const refetch = useCallback(async (): Promise<void> => {
		await loadConversation(chatId);
	}, [chatId, loadConversation]);

	return { chat, isLoading, error, refetch };
};
