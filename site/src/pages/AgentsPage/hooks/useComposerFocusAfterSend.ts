import { useEffect, useState } from "react";
import type { ChatMessageInputRef } from "../components/AgentChatInput";
import type { SubmitChatTurnParams } from "../components/ChatConversation/submitChatTurn";

type SendChatMessage = SubmitChatTurnParams["sendMessage"];

/**
 * Wraps a send so that the chat input takes focus when the send settles. The
 * chat input itself skips that refocus on narrow screens, so the on-screen
 * keyboard stays closed after a composer send.
 */
export const useComposerFocusAfterSend = (
	chatInputRef: React.RefObject<ChatMessageInputRef | null>,
	send: SendChatMessage,
): SendChatMessage => {
	const [focusRequest, setFocusRequest] = useState(0);
	// The editor accepts focus only in the render in which the finished send
	// enables it again.
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
