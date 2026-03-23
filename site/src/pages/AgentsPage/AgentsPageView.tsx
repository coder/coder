import type * as TypesGen from "api/typesGenerated";
import type { ModelSelectorOption } from "components/ai-elements";
import type { FC } from "react";
import { Outlet, useLocation } from "react-router";
import { cn } from "utils/cn";
import { pageTitle } from "utils/page";
import {
	AgentsSidebar,
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
	isSidebarCollapsed: boolean;
	onToggleSidebarCollapsed: () => void;
	onExpandSidebar: () => void;
}

interface AgentsPageViewProps {
	agentId: string | undefined;
	chatList: TypesGen.Chat[];
	catalogModelOptions: readonly ModelSelectorOption[];
	modelConfigs: readonly TypesGen.ChatModelConfig[];
	logoUrl: string;
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
	onToggleSidebarCollapsed: () => void;
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
	logoUrl,
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
	onToggleSidebarCollapsed,
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
	const isSettingsIndex =
		sidebarView.panel === "settings" && !sidebarView.section;
	const isSettingsDetail =
		sidebarView.panel === "settings" && Boolean(sidebarView.section);
	const isAnalytics = sidebarView.panel === "analytics";

	// The sidebar expects plain string error messages, but the outlet
	// context now carries structured ChatDetailError objects.
	const sidebarChatErrorReasons = Object.fromEntries(
		Object.entries(chatErrorReasons).map(([chatId, error]) => [
			chatId,
			error.message,
		]),
	);

	const outletContextValue: AgentsOutletContext = {
		chatErrorReasons,
		setChatErrorReason,
		clearChatErrorReason,
		requestArchiveAgent,
		requestUnarchiveAgent,
		requestArchiveAndDeleteWorkspace,
		isSidebarCollapsed,
		onToggleSidebarCollapsed,
		onExpandSidebar,
	};

	return (
		<div className="flex h-full min-h-0 flex-col overflow-hidden bg-surface-primary md:flex-row">
			<title>{pageTitle("Agents")}</title>
			<div
				className={cn(
					"md:h-full md:w-[320px] md:min-h-0 md:border-b-0",
					agentId
						? "hidden md:block shrink-0 h-[42dvh] min-h-[240px] border-b border-border-default"
						: isSettingsDetail || isAnalytics
							? "hidden md:block shrink-0"
							: "order-2 md:order-none flex-1 min-h-0 border-t border-border-default md:flex-none md:border-t-0",
					isSidebarCollapsed && "md:hidden",
				)}
			>
				<AgentsSidebar
					chats={chatList}
					chatErrorReasons={sidebarChatErrorReasons}
					modelOptions={catalogModelOptions}
					modelConfigs={modelConfigs}
					logoUrl={logoUrl}
					onArchiveAgent={requestArchiveAgent}
					onUnarchiveAgent={requestUnarchiveAgent}
					onArchiveAndDeleteWorkspace={requestArchiveAndDeleteWorkspace}
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
					isAdmin={isAgentsAdmin}
				/>
			</div>
			<div
				className={cn(
					"min-h-0 min-w-0 flex-1 flex-col bg-surface-primary",
					isSettingsIndex ? "hidden md:flex" : "flex",
					!agentId &&
						!isSettingsDetail &&
						sidebarView.panel === "chats" &&
						"order-1 md:order-none flex-none md:flex-1",
				)}
			>
				<Outlet context={outletContextValue} />
			</div>
		</div>
	);
};
