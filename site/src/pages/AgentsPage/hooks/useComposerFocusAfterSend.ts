import { useEffect, useState } from "react";
import type { ChatMessageInputRef } from "../components/AgentChatInput";
import type { SubmitChatTurnParams } from "../components/ChatConversation/submitChatTurn";

type SendChatMessage = SubmitChatTurnParams["sendMessage"];

/** Wraps an answer's send so that the chat input takes focus once it is sent. */
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
		let userMovedFocus = false;
		const noteUserMove = () => {
			userMovedFocus = true;
		};
		// A click, even on text that takes no focus, or a focus change while the
		// answer sends is the user's own move, and the chat input leaves it alone.
		document.addEventListener("pointerdown", noteUserMove, true);
		document.addEventListener("focusin", noteUserMove, true);
		const response = await send(request).finally(() => {
			document.removeEventListener("pointerdown", noteUserMove, true);
			document.removeEventListener("focusin", noteUserMove, true);
		});
		if (!userMovedFocus) {
			setFocusRequest((count) => count + 1);
		}
		return response;
	};
};
