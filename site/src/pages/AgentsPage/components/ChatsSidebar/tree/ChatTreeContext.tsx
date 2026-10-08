import { createContext, useContext } from "react";
import type { Chat, ChatModel } from "#/api/typesGenerated";
import type { ChatTree } from "./chatTree";

export type ChatTreeContextValue = {
	readonly chatTree: ChatTree;
	readonly chatById: ReadonlyMap<string, Chat>;
	readonly visibleChatIDs: ReadonlySet<string>;
	readonly normalizedSearch: string;
	readonly expandedById: Record<string, boolean>;
	readonly modelConfigs: readonly ChatModel[];
	readonly isLoadingModelConfigs: boolean;
	readonly chatErrorReasons: Record<string, string>;
	readonly activeChatId: string | undefined;
	readonly currentUserId: string;
	readonly toggleExpanded: (chatID: string) => void;
	readonly onArchiveSuccess?: (chatId: string) => void;
	readonly navigateAfterArchive: (chatId: string) => void;
	readonly onMarkChatRead: (chatId: string) => void;
	readonly onMarkChatUnread: (chatId: string) => void;
	readonly onOpenRenameDialog?: (chat: Chat) => void;
};

export const ChatTreeContext = createContext<ChatTreeContextValue | null>(null);

export function useChatTree(): ChatTreeContextValue {
	const ctx = useContext(ChatTreeContext);
	if (!ctx) {
		throw new Error("useChatTree must be used within ChatTreeContext.Provider");
	}
	return ctx;
}
