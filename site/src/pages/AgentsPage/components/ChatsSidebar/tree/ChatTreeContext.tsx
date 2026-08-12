import { createContext, useContext } from "react";
import type { Chat, ChatModelConfig } from "#/api/typesGenerated";
import type { ModelSelectorOption } from "../../ChatElements";
import type { ChatTree } from "./chatTree";

export interface ChatTreeContextValue {
	readonly chatTree: ChatTree;
	readonly chatById: ReadonlyMap<string, Chat>;
	readonly visibleChatIDs: ReadonlySet<string>;
	readonly normalizedSearch: string;
	readonly expandedById: Record<string, boolean>;
	readonly modelOptions: readonly ModelSelectorOption[];
	readonly modelConfigs: readonly ChatModelConfig[];
	readonly chatErrorReasons: Record<string, string>;
	readonly activeChatId: string | undefined;
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
}

export const ChatTreeContext = createContext<ChatTreeContextValue | null>(null);

export function useChatTree(): ChatTreeContextValue {
	const ctx = useContext(ChatTreeContext);
	if (!ctx) {
		throw new Error("useChatTree must be used within ChatTreeContext.Provider");
	}
	return ctx;
}
