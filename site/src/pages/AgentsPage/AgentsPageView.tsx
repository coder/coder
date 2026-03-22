import type * as TypesGen from "api/typesGenerated";
import type { ModelSelectorOption } from "components/ai-elements";
import { Button } from "components/Button/Button";
import { ExternalImage } from "components/ExternalImage/ExternalImage";
import { CoderIcon } from "components/Icons/CoderIcon";
import type { Dayjs } from "dayjs";
import { PanelLeftIcon } from "lucide-react";
import type { FC } from "react";
import { NavLink, Outlet, useLocation, useNavigate } from "react-router";
import { cn } from "utils/cn";
import { pageTitle } from "utils/page";
import { AgentCreateForm, type CreateChatOptions } from "./AgentCreateForm";
import { AgentsSidebar, sidebarViewFromPath } from "./AgentsSidebar";
import { AnalyticsPageContent } from "./AnalyticsPageContent";
import { ChimeButton } from "./ChimeButton";
import { SettingsPageContent } from "./SettingsPageContent";
import type { ChatDetailError } from "./usageLimitMessage";
import { WebPushButton } from "./WebPushButton";

type ChatModelOption = ModelSelectorOption;

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
	onOpenAnalytics?: () => void;
	modelOptions: readonly ModelSelectorOption[];
	modelConfigIDByModelID: ReadonlyMap<string, string>;
	modelIDByConfigID: ReadonlyMap<string, string>;
	modelConfigs: readonly TypesGen.ChatModelConfig[];
	modelCatalog: TypesGen.ChatModelsResponse | null | undefined;
	isModelCatalogLoading: boolean;
	modelCatalogError: unknown;
	desktopEnabled: boolean;
}

interface AgentsPageViewProps {
	agentId: string | undefined;
	chatList: TypesGen.Chat[];
	catalogModelOptions: readonly ChatModelOption[];
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
	onCreateChat: (options: CreateChatOptions) => Promise<void>;
	createError: unknown;
	modelCatalog: TypesGen.ChatModelsResponse | null | undefined;
	isModelCatalogLoading: boolean;
	isModelConfigsLoading: boolean;
	modelCatalogError: unknown;
	modelConfigIDByModelID: ReadonlyMap<string, string>;
	desktopEnabled: boolean;
	hasNextPage: boolean | undefined;
	onLoadMore: () => void;
	isFetchingNextPage: boolean;
	archivedFilter: "active" | "archived";
	onArchivedFilterChange: (filter: "active" | "archived") => void;
	analyticsNow?: Dayjs;
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
	onCreateChat,
	createError,
	modelCatalog,
	isModelCatalogLoading,
	isModelConfigsLoading,
	modelCatalogError,
	modelConfigIDByModelID,
	desktopEnabled,
	hasNextPage,
	onLoadMore,
	isFetchingNextPage,
	archivedFilter,
	onArchivedFilterChange,
	analyticsNow,
}) => {
	const location = useLocation();
	const navigate = useNavigate();
	const sidebarView = sidebarViewFromPath(location.pathname);

	const handleOpenAnalytics = () => {
		navigate("/agents/analytics");
	};

	// The sidebar expects plain string error messages, but the outlet
	// context now carries structured ChatDetailError objects.
	const sidebarChatErrorReasons = Object.fromEntries(
		Object.entries(chatErrorReasons).map(([chatId, error]) => [
			chatId,
			error.message,
		]),
	);

	const modelIDByConfigID = (() => {
		const byConfigID = new Map<string, string>();
		for (const [modelID, configID] of modelConfigIDByModelID.entries()) {
			if (!byConfigID.has(configID)) {
				byConfigID.set(configID, modelID);
			}
		}
		return byConfigID;
	})();

	const outletContextValue: AgentsOutletContext = {
		chatErrorReasons,
		setChatErrorReason,
		clearChatErrorReason,
		requestArchiveAgent,
		requestUnarchiveAgent,
		requestArchiveAndDeleteWorkspace,
		isSidebarCollapsed,
		onToggleSidebarCollapsed,
		onOpenAnalytics: handleOpenAnalytics,
		modelOptions: catalogModelOptions,
		modelConfigIDByModelID,
		modelIDByConfigID,
		modelConfigs,
		modelCatalog,
		isModelCatalogLoading,
		modelCatalogError,
		desktopEnabled,
	};

	return (
		<div className="flex h-full min-h-0 flex-col overflow-hidden bg-surface-primary md:flex-row">
			<title>{pageTitle("Agents")}</title>
			<div
				className={cn(
					"md:h-full md:w-[320px] md:min-h-0 md:border-b-0",
					agentId
						? "hidden md:block shrink-0 h-[42dvh] min-h-[240px] border-b border-border-default"
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
					"flex min-h-0 min-w-0 flex-1 flex-col bg-surface-primary",
					!agentId &&
						sidebarView.panel === "chats" &&
						"order-1 md:order-none flex-none md:flex-1",
				)}
			>
				{sidebarView.panel === "settings" ? (
					<SettingsPageContent
						activeSection={sidebarView.section}
						canManageChatModelConfigs={isAgentsAdmin}
						canSetSystemPrompt={isAgentsAdmin}
					/>
				) : sidebarView.panel === "analytics" ? (
					<AnalyticsPageContent now={analyticsNow} />
				) : agentId ? (
					<Outlet key={agentId} context={outletContextValue} />
				) : (
					<>
						<div className="flex shrink-0 items-center gap-2 px-4 py-0.5">
							<NavLink
								to="/workspaces"
								className="inline-flex shrink-0 md:hidden"
							>
								{logoUrl ? (
									<ExternalImage className="h-6" src={logoUrl} alt="Logo" />
								) : (
									<CoderIcon className="h-6 w-6 fill-content-primary" />
								)}
							</NavLink>
							{isSidebarCollapsed && (
								<Button
									variant="subtle"
									size="icon"
									onClick={onExpandSidebar}
									aria-label="Expand sidebar"
									className="hidden h-7 w-7 min-w-0 shrink-0 md:inline-flex"
								>
									<PanelLeftIcon />
								</Button>
							)}
							<div className="flex min-w-0 flex-1 items-center" />
							<div className="flex items-center gap-2">
								<ChimeButton />
								<WebPushButton />
							</div>
						</div>
						<AgentCreateForm
							onCreateChat={onCreateChat}
							isCreating={isCreating}
							createError={createError}
							modelCatalog={modelCatalog}
							modelOptions={catalogModelOptions}
							modelConfigs={modelConfigs}
							isModelCatalogLoading={isModelCatalogLoading}
							isModelConfigsLoading={isModelConfigsLoading}
							modelCatalogError={modelCatalogError}
							onOpenAnalytics={handleOpenAnalytics}
						/>
					</>
				)}
			</div>
		</div>
	);
};
