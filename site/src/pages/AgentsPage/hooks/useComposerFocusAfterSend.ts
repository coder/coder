import { useEffect, useState } from "react";
import type { ChatMessageInputRef } from "../components/AgentChatInput";
import type { SubmitChatTurnParams } from "../components/ChatConversation/submitChatTurn";

type SendChatMessage = SubmitChatTurnParams["sendMessage"];

/**
 * Focuses the chat input once the wrapped send settles, also on narrow
 * screens, where the chat input skips its own refocus to keep the on-screen
 * keyboard closed.
 */
export const useComposerFocusAfterSend = (
	chatInputRef: React.RefObject<ChatMessageInputRef | null>,
	send: SendChatMessage,
): SendChatMessage => {
	const [focusRequest, setFocusRequest] = useState(0);
	// The editor refuses focus while the send keeps it disabled, so the focus
	// waits for the commit that enables it again.
	useEffect(() => {
		if (focusRequest > 0) {
			chatInputRef.current?.focus();
		}
	}, [focusRequest, chatInputRef]);

	// Promise.finally, because the React Compiler cannot compile a try statement
	// that has no catch clause.
	return (request) =>
		send(request).finally(() => {
			setFocusRequest((count) => count + 1);
		});
};
