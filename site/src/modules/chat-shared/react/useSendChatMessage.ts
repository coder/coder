import { useCallback, useState } from "react";
import { useChatRuntimeContext } from "./ChatRuntimeProvider";
import { useChatPreferences } from "./useChatPreferences";

/** @public Input for the shared chat message sender hook. */
export type SendChatMessageInput = {
	message: string;
	parentMessageId?: number;
};

/** @public Result state for the shared chat message sender hook. */
export type UseSendChatMessageResult = {
	sendMessage: (input: SendChatMessageInput) => Promise<void>;
	isSending: boolean;
	lastError: unknown;
};

/** @public Sends a message through the shared chat runtime. */
export const useSendChatMessage = (): UseSendChatMessageResult => {
	const { runtime, store, activeChatId } = useChatRuntimeContext();
	const { selectedModel } = useChatPreferences();
	const [isSending, setIsSending] = useState(false);
	const [lastError, setLastError] = useState<unknown>(null);

	const sendMessage = useCallback(
		async ({
			message,
			parentMessageId,
		}: SendChatMessageInput): Promise<void> => {
			if (!activeChatId) {
				throw new Error("Cannot send a chat message without an active chat.");
			}

			setLastError(null);
			setIsSending(true);
			try {
				const result = await runtime.sendMessage({
					chatId: activeChatId,
					message,
					model: selectedModel || undefined,
					parentMessageId,
				});
				if (result.queued) {
					if (!result.queued_message) {
						throw new Error(
							"Queued chat responses must include a queued message.",
						);
					}
					const queuedMessages = store.getSnapshot().queuedMessages;
					store.setQueuedMessages([...queuedMessages, result.queued_message]);
					return;
				}
				if (!result.message) {
					throw new Error("Durable chat responses must include a message.");
				}
				store.upsertDurableMessage(result.message);
			} catch (error) {
				setLastError(error);
				throw error;
			} finally {
				setIsSending(false);
			}
		},
		[activeChatId, runtime, selectedModel, store],
	);

	return { sendMessage, isSending, lastError };
};
