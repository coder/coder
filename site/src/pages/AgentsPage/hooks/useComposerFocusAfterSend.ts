import { useEffect, useState } from "react";
import type { ChatMessageInputRef } from "../components/AgentChatInput";
import type { SubmitChatTurnParams } from "../components/ChatConversation/submitChatTurn";

type SendChatMessage = SubmitChatTurnParams["sendMessage"];

/**
 * Wraps the send of a question answer, whose form closes and drops focus, so
 * that the chat input takes focus once it settles. This also covers narrow
 * screens, where the chat input skips its own refocus after a send so that
 * the on-screen keyboard stays closed.
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
