import type { Chat, ChatModelConfig } from "#/api/typesGenerated";
import type { ModelSelectorOption } from "../ChatElements";

export interface ChatsSidebarProps {
	chats: readonly Chat[];
	chatErrorReasons: Record<string, string>;
	modelOptions: readonly ModelSelectorOption[];
	modelConfigs: readonly ChatModelConfig[];
	onArchiveAgent: (chatId: string) => void;
	onUnarchiveAgent: (chatId: string) => void;
	onArchiveAndDeleteWorkspace: (chatId: string, workspaceId: string) => void;
	onPinAgent: (chatId: string) => void;
	onUnpinAgent: (chatId: string) => void;
	onReorderPinnedAgent?: (chatId: string, pinOrder: number) => void;
	onRenameTitle?: (chatId: string, title: string) => Promise<void>;
	onProposeTitle?: (chatId: string) => Promise<string>;
	onBeforeNewAgent?: () => void;
	isCreating: boolean;
	isArchiving?: boolean;
	archivingChatId?: string | null;
	regeneratingTitleChatIds: readonly string[];
	isLoading?: boolean;
	loadError?: unknown;
	onRetryLoad?: () => void;
	hasNextPage?: boolean;
	onLoadMore?: () => void;
	isFetchingNextPage?: boolean;
	archivedFilter: "active" | "archived";
	onArchivedFilterChange?: (filter: "active" | "archived") => void;
	onCollapse?: () => void;
	isPersonalModelOverridesEnabled?: boolean;
	isAdmin?: boolean;
}
