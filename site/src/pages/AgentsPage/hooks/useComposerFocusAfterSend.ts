import { useEffect, useState } from "react";
import type { ChatMessageInputRef } from "../components/AgentChatInput";

/**
 * Wraps a send so that the chat input takes focus when the send settles, on
 * every screen size. The chat input itself skips that refocus on narrow
 * screens, so the on-screen keyboard stays closed after a composer send.
 */
export const useComposerFocusAfterSend = <Request, Response>(
	chatInputRef: React.RefObject<ChatMessageInputRef | null>,
	send: (request: Request) => Promise<Response>,
): ((request: Request) => Promise<Response>) => {
	const [focusRequest, setFocusRequest] = useState(0);
	// The editor accepts focus only in the render in which the finished send
	// enables it again.
	useEffect(() => {
		if (focusRequest > 0) {
			chatInputRef.current?.focus();
		}
	}, [focusRequest, chatInputRef]);

	// Promise.finally, because the React Compiler cannot compile a try
	// statement that has no catch clause.
	return (request) =>
		send(request).finally(() => {
			setFocusRequest((count) => count + 1);
		});
};
