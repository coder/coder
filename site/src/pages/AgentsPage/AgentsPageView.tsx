import type * as TypesGen from "api/typesGenerated";
import type { ModelSelectorOption } from "components/ai-elements";
import { Button } from "components/Button/Button";
import { ExternalImage } from "components/ExternalImage/ExternalImage";
import { CoderIcon } from "components/Icons/CoderIcon";
import { PanelLeftIcon } from "lucide-react";
import { type FC, useCallback, useMemo, useState } from "react";
import { NavLink, Outlet } from "react-router";
import { cn } from "utils/cn";
import { pageTitle } from "utils/page";
import { AgentCreateForm, type CreateChatOptions } from "./AgentCreateForm";
import { AgentsSidebar } from "./AgentsSidebar";
import { ChimeButton } from "./ChimeButton";
import {
	ConfigureAgentsDialog,
	type ConfigureAgentsSection,
} from "./ConfigureAgentsDialog";
import { UserAnalyticsDialog } from "./UserAnalyticsDialog";
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
	onOpenAnalytics?: () => void;
	isSidebarCollapsed: boolean;
	onToggleSidebarCollapsed: () => void;
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
	outletContext: AgentsOutletContext;
	isAgentsAdmin: boolean;
	onCreateChat: (options: CreateChatOptions) => Promise<void>;
	createError: unknown;
	modelCatalog: TypesGen.ChatModelsResponse | null | undefined;
	isModelCatalogLoading: boolean;
	isModelConfigsLoading: boolean;
	modelCatalogError: unknown;
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
	outletContext,
	isAgentsAdmin,
	onCreateChat,
	createError,
	modelCatalog,
	isModelCatalogLoading,
	isModelConfigsLoading,
	modelCatalogError,
	hasNextPage,
	onLoadMore,
	isFetchingNextPage,
	archivedFilter,
	onArchivedFilterChange,
}) => {
	const {
		chatErrorReasons,
		requestArchiveAgent,
		requestUnarchiveAgent,
		requestArchiveAndDeleteWorkspace,
	} = outletContext;
	const [isConfigureAgentsDialogOpen, setConfigureAgentsDialogOpen] =
		useState(false);
	const [configDialogKey, setConfigDialogKey] = useState(0);
	const [configureAgentsInitialSection, setConfigureAgentsInitialSection] =
		useState<ConfigureAgentsSection>("behavior");
	const [isUserAnalyticsDialogOpen, setUserAnalyticsDialogOpen] =
		useState(false);
	const [analyticsDialogKey, setAnalyticsDialogKey] = useState(0);
	const handleOpenAnalytics = useCallback(() => {
		setAnalyticsDialogKey((key) => key + 1);
		setUserAnalyticsDialogOpen(true);
	}, []);
	const sidebarChatErrorReasons = useMemo(
		() =>
			Object.fromEntries(
				Object.entries(chatErrorReasons).map(([chatId, error]) => [
					chatId,
					error.message,
				]),
			),
		[chatErrorReasons],
	);
	const outletContextValue = useMemo(
		() => ({ ...outletContext, onOpenAnalytics: handleOpenAnalytics }),
		[outletContext, handleOpenAnalytics],
	);
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
					onNewAgent={handleNewAgent}
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
					onOpenAnalytics={handleOpenAnalytics}
					onOpenSettings={() => {
						setConfigureAgentsInitialSection("behavior");
						setConfigDialogKey((key) => key + 1);
						setConfigureAgentsDialogOpen(true);
					}}
				/>
			</div>

			<div
				className={cn(
					"flex min-h-0 min-w-0 flex-1 flex-col bg-surface-primary",
					!agentId && "order-1 md:order-none flex-none md:flex-1",
				)}
			>
				{agentId ? (
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

			<ConfigureAgentsDialog
				key={configDialogKey}
				open={isConfigureAgentsDialogOpen}
				onOpenChange={setConfigureAgentsDialogOpen}
				canManageChatModelConfigs={isAgentsAdmin}
				canSetSystemPrompt={isAgentsAdmin}
				initialSection={configureAgentsInitialSection}
			/>
			<UserAnalyticsDialog
				key={analyticsDialogKey}
				open={isUserAnalyticsDialogOpen}
				onOpenChange={setUserAnalyticsDialogOpen}
			/>
		</div>
	);
};
