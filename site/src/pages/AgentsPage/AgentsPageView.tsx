import { type FC, type RefObject, useRef, useState } from "react";
import { Outlet, useLocation } from "react-router";
import type * as TypesGen from "#/api/typesGenerated";
import { cn } from "#/utils/cn";
import { pageTitle } from "#/utils/page";
import type { ModelSelectorOption } from "./components/ChatElements";
import {
	ChatsSidebar,
	isSettingsView,
	sidebarViewFromPath,
} from "./components/ChatsSidebar/ChatsSidebar";
import { ResizableChatsSidebarFrame } from "./components/ChatsSidebar/ResizableChatsSidebarFrame";
import type { AgentSidebarFilters } from "./utils/agentSidebarFilters";
import type { ChatDetailError } from "./utils/usageLimitMessage";

export interface AgentsOutletContext {
	chatErrorReasons: Record<string, ChatDetailError>;
	setChatErrorReason: (chatId: string, reason: ChatDetailError) => void;
	clearChatErrorReason: (chatId: string) => void;
	requestArchiveAgent: (chatId: string) => void;
	requestUnarchiveAgent: (chatId: string) => void;
	requestArchiveAndDeleteWorkspace: (
		chatId: string,
		workspaceId: string,
	) => void;
	requestPinAgent: (chatId: string) => void;
	requestUnpinAgent: (chatId: string) => void;
	requestReorderPinnedAgent?: (chatId: string, pinOrder: number) => void;
	isArchiving: boolean;
	archivingChatId: string | undefined;
	onRenameTitle?: (chatId: string, title: string) => Promise<void>;
	/** Opens the shared rename dialog so both menus drive the same instance. */
	onOpenRenameDialog?: (chat: TypesGen.Chat) => void;
	isSidebarCollapsed: boolean;
	onToggleSidebarCollapsed: () => void;
	onExpandSidebar: () => void;
	onChatReady: () => void;
	/** Ref attached to the chat scroll container by AgentChatPage. */
	scrollContainerRef: RefObject<HTMLDivElement | null>;
}

interface AgentsPageViewProps {
	agentId: string | undefined;
	chatList: TypesGen.Chat[];
	currentUserId: string;
	catalogModelOptions: readonly ModelSelectorOption[];
	modelConfigs: readonly TypesGen.ChatModelConfig[];
	handleNewAgent: () => void;
	isSearchDialogOpen: boolean;
	onSearchDialogOpenChange: (open: boolean) => void;
	isCreating: boolean;
	isArchiving: boolean;
	archivingChatId: string | undefined;
	isChatsLoading: boolean;
	chatsLoadError: Error | null;
	onRetryChatsLoad: () => void;
	onCollapseSidebar: () => void;
	isSidebarCollapsed: boolean;
	onExpandSidebar: () => void;
	chatErrorReasons: Record<string, ChatDetailError>;
	setChatErrorReason: (chatId: string, reason: ChatDetailError) => void;
	clearChatErrorReason: (chatId: string) => void;
	requestArchiveAgent: (chatId: string) => void;
	requestUnarchiveAgent: (chatId: string) => void;
	requestArchiveAndDeleteWorkspace: (
		chatId: string,
		workspaceId: string,
	) => void;
	requestPinAgent: (chatId: string) => void;
	requestUnpinAgent: (chatId: string) => void;
	requestReorderPinnedAgent?: (chatId: string, pinOrder: number) => void;
	onProposeTitle: (chatId: string) => Promise<string>;
	onRenameTitle: (chatId: string, title: string) => Promise<void>;
	onToggleSidebarCollapsed: () => void;
	isPersonalModelOverridesEnabled?: boolean;
	isAgentsAdmin: boolean;
	hasNextPage: boolean | undefined;
	onLoadMore: () => void;
	isFetchingNextPage: boolean;
	sidebarFilters: AgentSidebarFilters;
	onSidebarFiltersChange: (filters: AgentSidebarFilters) => void;
}

export const AgentsPageView: FC<AgentsPageViewProps> = ({
	agentId,
	chatList,
	currentUserId,
	catalogModelOptions,
	modelConfigs,
	handleNewAgent,
	isSearchDialogOpen,
	onSearchDialogOpenChange,
	isCreating,
	isArchiving,
	archivingChatId,
	isChatsLoading,
	chatsLoadError,
	onRetryChatsLoad,
	onCollapseSidebar,
	isSidebarCollapsed,
	onExpandSidebar,
	chatErrorReasons,
	setChatErrorReason,
	clearChatErrorReason,
	requestArchiveAgent,
	requestUnarchiveAgent,
	requestArchiveAndDeleteWorkspace,
	requestPinAgent,
	requestUnpinAgent,
	requestReorderPinnedAgent,
	onProposeTitle,
	onRenameTitle,
	onToggleSidebarCollapsed,
	isPersonalModelOverridesEnabled,
	isAgentsAdmin,
	hasNextPage,
	onLoadMore,
	isFetchingNextPage,
	sidebarFilters,
	onSidebarFiltersChange,
}) => {
	const location = useLocation();
	const sidebarView = sidebarViewFromPath(location.pathname);

	// Mobile can't fit the sidebar nav and content side by side,
	// so we show one or the other depending on the route depth.
	const isSettingsPanel = isSettingsView(sidebarView);
	const isSettingsIndex = isSettingsPanel && !sidebarView.section;
	const isSettingsDetail = isSettingsPanel && Boolean(sidebarView.section);
	const isAnalytics = sidebarView.panel === "analytics";

	// The sidebar expects plain string error messages, but the outlet
	// context now carries structured ChatDetailError objects.
	const sidebarChatErrorReasons = Object.fromEntries(
		Object.entries(chatErrorReasons).map(([chatId, error]) => [
			chatId,
			error.message,
		]),
	);

	const scrollContainerRef = useRef<HTMLDivElement | null>(null);

	// State for the shared rename-chat dialog. Lifted here so both the
	// sidebar menu and the chat top bar open the same dialog instance.
	const [chatPendingRename, setChatPendingRename] =
		useState<TypesGen.Chat | null>(null);

	const outletContextValue: AgentsOutletContext = {
		chatErrorReasons,
		setChatErrorReason,
		clearChatErrorReason,
		requestArchiveAgent,
		requestUnarchiveAgent,
		requestArchiveAndDeleteWorkspace,
		requestPinAgent,
		requestUnpinAgent,
		requestReorderPinnedAgent,
		isArchiving,
		archivingChatId,
		onOpenRenameDialog: setChatPendingRename,
		isSidebarCollapsed,
		onToggleSidebarCollapsed,
		onExpandSidebar,
		onChatReady: () => {},
		scrollContainerRef,
	};

	return (
		<div
			data-testid="agents-page-layout"
			className="flex h-full min-h-0 flex-col overflow-hidden bg-surface-primary sm:flex-row"
		>
			<title>{pageTitle("Agents")}</title>
			<ResizableChatsSidebarFrame
				className={cn(
					"sm:h-full sm:min-h-0 sm:border-b-0",
					agentId
						? "hidden sm:block shrink-0 h-[42dvh] min-h-[240px] border-b border-border-default"
						: isSettingsDetail || isAnalytics
							? "hidden sm:block shrink-0"
							: "order-2 sm:order-none flex-1 min-h-0 border-b border-border-default sm:flex-none sm:border-t-0 sm:border-b-0",
					isSidebarCollapsed && "sm:hidden",
				)}
			>
				<ChatsSidebar
					chats={chatList}
					currentUserId={currentUserId}
					chatErrorReasons={sidebarChatErrorReasons}
					modelOptions={catalogModelOptions}
					modelConfigs={modelConfigs}
					onArchiveAgent={requestArchiveAgent}
					onUnarchiveAgent={requestUnarchiveAgent}
					onArchiveAndDeleteWorkspace={requestArchiveAndDeleteWorkspace}
					onPinAgent={requestPinAgent}
					onUnpinAgent={requestUnpinAgent}
					onReorderPinnedAgent={requestReorderPinnedAgent}
					onRenameTitle={onRenameTitle}
					onProposeTitle={onProposeTitle}
					chatPendingRename={chatPendingRename}
					onChatPendingRenameChange={setChatPendingRename}
					onBeforeNewAgent={handleNewAgent}
					isSearchDialogOpen={isSearchDialogOpen}
					onSearchDialogOpenChange={onSearchDialogOpenChange}
					isCreating={isCreating}
					isArchiving={isArchiving}
					archivingChatId={archivingChatId}
					isLoading={isChatsLoading}
					loadError={chatsLoadError}
					onRetryLoad={onRetryChatsLoad}
					hasNextPage={hasNextPage}
					onLoadMore={onLoadMore}
					isFetchingNextPage={isFetchingNextPage}
					sidebarFilters={sidebarFilters}
					onSidebarFiltersChange={onSidebarFiltersChange}
					onCollapse={onCollapseSidebar}
					isPersonalModelOverridesEnabled={isPersonalModelOverridesEnabled}
					isAdmin={isAgentsAdmin}
				/>
			</ResizableChatsSidebarFrame>
			<div
				data-testid="agents-main-panel"
				className={cn(
					"min-h-0 min-w-0 flex-1 flex-col bg-surface-primary",
					isSettingsIndex ? "hidden sm:flex" : "flex",
					!agentId &&
						!isSettingsDetail &&
						sidebarView.panel === "chats" &&
						"contents sm:flex sm:flex-1 sm:flex-col",
				)}
			>
				<Outlet context={outletContextValue} />
			</div>
		</div>
	);
};
