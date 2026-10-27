import { useEffect, useState } from "react";
import type { ChatMessageInputRef } from "../components/AgentChatInput";
import type { SubmitChatTurnParams } from "../components/ChatConversation/submitChatTurn";

type SendChatMessage = SubmitChatTurnParams["sendMessage"];

/**
 * Wraps a question answer's send so that the chat input takes focus once the
 * answer is sent and its form closes. That includes narrow screens, where the
 * chat input skips its own refocus to keep the on-screen keyboard closed.
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

	return async (request) => {
		const focusedAtSend = document.activeElement;
		const response = await send(request);
		// Focus the user moved elsewhere during the send stays there.
		if (
			document.activeElement === focusedAtSend ||
			document.activeElement === document.body
		) {
			setFocusRequest((count) => count + 1);
		}
		return response;
	};
};
