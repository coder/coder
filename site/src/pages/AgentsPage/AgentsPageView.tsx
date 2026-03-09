import type * as TypesGen from "api/typesGenerated";
import type { ModelSelectorOption } from "components/ai-elements";
import { Button } from "components/Button/Button";
import { ExternalImage } from "components/ExternalImage/ExternalImage";
import { CoderIcon } from "components/Icons/CoderIcon";
import { PanelLeftIcon } from "lucide-react";
import type { FC, ReactNode } from "react";
import { NavLink, Outlet } from "react-router";
import { cn } from "utils/cn";
import { pageTitle } from "utils/page";

import { AgentsSidebar } from "./AgentsSidebar";
import { ChimeButton } from "./ChimeButton";
import { WebPushButton } from "./WebPushButton";

type ChatModelOption = ModelSelectorOption;

export interface AgentsOutletContext {
	chatErrorReasons: Record<string, string>;
	setChatErrorReason: (chatId: string, reason: string) => void;
	clearChatErrorReason: (chatId: string) => void;
	requestArchiveAgent: (chatId: string) => void;
	requestUnarchiveAgent: (chatId: string) => void;
	requestArchiveAndDeleteWorkspace: (
		chatId: string,
		workspaceId: string,
	) => void;
	isSidebarCollapsed: boolean;
	onToggleSidebarCollapsed: () => void;
}

interface AgentsPageViewProps {
	agentId: string | undefined;
	chatList: TypesGen.Chat[];
	chatErrorReasons: Record<string, string>;
	catalogModelOptions: readonly ChatModelOption[];
	modelConfigs: readonly TypesGen.ChatModelConfig[];
	logoUrl: string;
	requestArchiveAgent: (chatId: string) => void;
	requestUnarchiveAgent: (chatId: string) => void;
	requestArchiveAndDeleteWorkspace: (
		chatId: string,
		workspaceId: string,
	) => void;
	handleNewAgent: () => void;
	isCreating: boolean;
	isArchiving: boolean;
	archivingChatId: string | undefined;
	isChatsLoading: boolean;
	chatsLoadError: Error | undefined;
	onRetryChatsLoad: () => void;
	onCollapseSidebar: () => void;
	isSidebarCollapsed: boolean;
	onExpandSidebar: () => void;
	outletContext: AgentsOutletContext;
	emptyStateNode: ReactNode;
	toolbarEndContent?: ReactNode;
}

export const AgentsPageView: FC<AgentsPageViewProps> = ({
	agentId,
	chatList,
	chatErrorReasons,
	catalogModelOptions,
	modelConfigs,
	logoUrl,
	requestArchiveAgent,
	requestUnarchiveAgent,
	requestArchiveAndDeleteWorkspace,
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
	emptyStateNode,
	toolbarEndContent,
}) => {
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
					chatErrorReasons={chatErrorReasons}
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
					onCollapse={onCollapseSidebar}
				/>
			</div>

			<div
				className={cn(
					"flex min-h-0 min-w-0 flex-1 flex-col bg-surface-primary",
					!agentId && "order-1 md:order-none flex-none md:flex-1",
				)}
			>
				{agentId ? (
					<Outlet key={agentId} context={outletContext} />
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
								{toolbarEndContent}
							</div>
						</div>
						{emptyStateNode}
					</>
				)}
			</div>
		</div>
	);
};
