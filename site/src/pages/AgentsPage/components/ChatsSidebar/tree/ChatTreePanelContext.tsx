import { createContext, useContext } from "react";
import type { Chat, ChatModel } from "#/api/typesGenerated";
import type { ChatTreeModel, ChatTreeVisibleRow } from "./chatTreeModel";

export type SubagentLoadState = "pending" | "error";

export interface ChatTreePanelContextValue {
	readonly model: ChatTreeModel;
	readonly rowsById: ReadonlyMap<string, ChatTreeVisibleRow>;
	readonly focusedId: string | undefined;
	readonly activeChatId: string | undefined;
	/** Rows kept only as a path to a filter match; rendered dimmed. */
	readonly dimmedIds: ReadonlySet<string>;
	/** Rows expanded by a filter; Left moves to the parent instead of collapsing. */
	readonly forceExpandedIds: ReadonlySet<string>;
	readonly subagentsShownIds: ReadonlySet<string>;
	/** Nodes whose subagent fetch has not produced children yet. */
	readonly subagentLoadStates: ReadonlyMap<string, SubagentLoadState>;
	readonly modelConfigs: readonly ChatModel[];
	readonly isLoadingModelConfigs: boolean;
	readonly chatErrorReasons: Record<string, string>;
	readonly isArchiving: boolean;
	readonly archivingChatId: string | null;
	/** Prefix for treeitem DOM ids so focus can be moved by id. */
	readonly treeDomId: string;
	readonly setFocusedId: (id: string) => void;
	readonly toggleExpanded: (id: string) => void;
	readonly toggleSubagents: (id: string) => void;
	readonly requestArchive: (chat: Chat) => void;
	readonly requestUnarchive: (chat: Chat) => void;
	readonly requestArchiveAndDeleteWorkspace: (
		chat: Chat,
		workspaceId: string,
	) => void;
	readonly onPinAgent: (chatId: string) => void;
	readonly onUnpinAgent: (chatId: string) => void;
	readonly onOpenRenameDialog?: (chat: Chat) => void;
	readonly onCreateChildChat: (chat: Chat) => void;
}

export const ChatTreePanelContext =
	createContext<ChatTreePanelContextValue | null>(null);

export function useChatTreePanel(): ChatTreePanelContextValue {
	const ctx = useContext(ChatTreePanelContext);
	if (!ctx) {
		throw new Error(
			"useChatTreePanel must be used within ChatTreePanelContext",
		);
	}
	return ctx;
}
