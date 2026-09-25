import { createContext, type ReactNode, useContext } from "react";
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
	readonly isArchiving: boolean;
	readonly archivingChatId: string | null;
	readonly toggleExpanded: (chatID: string) => void;
	readonly onArchiveAgent: (chatId: string) => void;
	readonly onUnarchiveAgent: (chatId: string) => void;
	readonly onArchiveAndDeleteWorkspace: (
		chatId: string,
		workspaceId: string,
	) => void;
	readonly onPinAgent: (chatId: string) => void;
	readonly onUnpinAgent: (chatId: string) => void;
	readonly onOpenRenameDialog?: (chat: Chat) => void;
	/** Extra content under the age in a row's right column. Absent unless an experiment supplies it. */
	readonly renderTrailing?: (chat: Chat) => ReactNode;
};

export const ChatTreeContext = createContext<ChatTreeContextValue | null>(null);

export function useChatTree(): ChatTreeContextValue {
	const ctx = useContext(ChatTreeContext);
	if (!ctx) {
		throw new Error("useChatTree must be used within ChatTreeContext.Provider");
	}
	return ctx;
}
