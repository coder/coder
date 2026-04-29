import { useContext, useSyncExternalStore } from "react";
import type { ChatSession } from "./ChatSession";
import type { ChatSessionManager } from "./ChatSessionManager";
import { InternalChatSessionsContext } from "./ChatSessionsProvider";
import type { ChatSessionSnapshot } from "./types";

export const useChatSessionsManager = (): ChatSessionManager => {
	const manager = useContext(InternalChatSessionsContext);
	if (!manager) {
		throw new Error(
			"useChatSessionsManager must be used inside <ChatSessionsProvider>",
		);
	}
	return manager;
};

export const useChatSession = (chatId: string): ChatSession => {
	const manager = useChatSessionsManager();
	return manager.getOrCreate(chatId);
};

export const useChatSessionSelector = <T>(
	chatId: string,
	selector: (snapshot: ChatSessionSnapshot) => T,
): T => {
	const session = useChatSession(chatId);
	return useSyncExternalStore(
		(listener) => session.subscribe(listener),
		() => selector(session.getSnapshot()),
		() => selector(session.getSnapshot()),
	);
};
