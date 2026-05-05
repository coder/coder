import { type FC, type RefObject, useRef } from "react";
import { Outlet, useLocation } from "react-router";
import type * as TypesGen from "#/api/typesGenerated";
import { cn } from "#/utils/cn";
import { pageTitle } from "#/utils/page";
import type { ModelSelectorOption } from "./components/ChatElements";
import {
	AgentsSidebar,
	isSettingsView,
	sidebarViewFromPath,
} from "./components/Sidebar/AgentsSidebar";
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
	onRegenerateTitle?: (chatId: string) => void;
	onRenameTitle?: (chatId: string, title: string) => Promise<void>;
	regeneratingTitleChatIds: readonly string[];
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
	catalogModelOptions: readonly ModelSelectorOption[];
	modelConfigs: readonly TypesGen.ChatModelConfig[];
	handleNewAgent: () => void;
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
	onRegenerateTitle: (chatId: string) => Promise<string>;
	onProposeTitle: (chatId: string) => Promise<string>;
	onRenameTitle: (chatId: string, title: string) => Promise<void>;
	regeneratingTitleChatIds: readonly string[];
	onToggleSidebarCollapsed: () => void;
	isPersonalModelOverridesEnabled?: boolean;
	isAgentsAdmin: boolean;
	hasNextPage: boolean | undefined;
	onLoadMore: () => void;
	isFetchingNextPage: boolean;
	archivedFilter: "active" | "archived";
	onArchivedFilterChange: (filter: "active" | "archived") => void;
}

export const AgentsPageView: FC<AgentsPageViewProps> = ({
	agentId,
	chatList,
	catalogModelOptions,
	modelConfigs,
	handleNewAgent,
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
	onRegenerateTitle,
	onProposeTitle,
	onRenameTitle,
	regeneratingTitleChatIds,
	onToggleSidebarCollapsed,
	isPersonalModelOverridesEnabled,
	isAgentsAdmin,
	hasNextPage,
	onLoadMore,
	isFetchingNextPage,
	archivedFilter,
	onArchivedFilterChange,
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
		onRegenerateTitle: (chatId: string) => {
			onRegenerateTitle(chatId).catch(() => {});
		},
		regeneratingTitleChatIds,
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
			<div
				data-testid="agents-sidebar-panel"
				className={cn(
					"sm:h-full sm:w-[320px] sm:min-h-0 sm:border-b-0",
					agentId
						? "hidden sm:block shrink-0 h-[42dvh] min-h-[240px] border-b border-border-default"
						: isSettingsDetail || isAnalytics
							? "hidden sm:block shrink-0"
							: "order-2 sm:order-none flex-1 min-h-0 border-b border-border-default sm:flex-none sm:border-t-0 sm:border-b-0",
					isSidebarCollapsed && "sm:hidden",
				)}
			>
				<AgentsSidebar
					chats={chatList}
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
					regeneratingTitleChatIds={regeneratingTitleChatIds}
					onBeforeNewAgent={handleNewAgent}
					isCreating={isCreating}
					isArchiving={isArchiving}
					archivingChatId={archivingChatId}
					isLoading={isChatsLoading}
					loadError={chatsLoadError}
					onRetryLoad={onRetryChatsLoad}
					hasNextPage={hasNextPage}
					onLoadMore={onLoadMore}
					isFetchingNextPage={isFetchingNextPage}
					archivedFilter={archivedFilter}
					onArchivedFilterChange={onArchivedFilterChange}
					onCollapse={onCollapseSidebar}
					isPersonalModelOverridesEnabled={isPersonalModelOverridesEnabled}
					isAdmin={isAgentsAdmin}
				/>
			</div>
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
