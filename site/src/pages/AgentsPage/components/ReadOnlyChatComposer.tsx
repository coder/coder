import { cn } from "cn";
import { InfoIcon } from "lucide-react";
import { useState } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import type { ModelSelectorOption } from "#/modules/aiModels/ModelSelector";
import { ProviderIcon } from "#/modules/aiModels/ProviderIcon";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { useChatDropdownPosition } from "../hooks/useChatDropdownPosition";
import { chatWidthClass, useChatFullWidth } from "../hooks/useChatFullWidth";
import {
	AgentSetupNotice,
	shouldShowAgentSetupNotice,
} from "./AgentSetupNotice";
import { type AttachedWorkspaceInfo, ChatToolBadges } from "./ChatToolBadges";
import {
	type AgentContextUsage,
	ContextUsageIndicator,
} from "./ContextUsageIndicator";
import { MCPServersMenuContent } from "./MCPServersMenuContent";
import { getActiveMCPServers } from "./mcpMenuServers";
import { QueuedMessagesList } from "./QueuedMessagesList";

type ReadOnlyChatComposerProps = {
	chat: TypesGen.Chat;
	model: ModelSelectorOption | undefined;
	modelSelectorPlaceholder: string;
	isModelCatalogLoading: boolean;
	contextUsage?: AgentContextUsage;
	onRefreshContext?: () => void;
	isRefreshingContext?: boolean;
	queuedMessages: readonly TypesGen.ChatQueuedMessage[];
	queuedMessageUnderEditID: number | null;
	isChatPaused: boolean;
	mcpServers: readonly TypesGen.MCPServerConfig[];
	selectedMCPServerIds: readonly string[];
	workspace?: TypesGen.Workspace;
	workspaceAgent?: TypesGen.WorkspaceAgent;
	attachedWorkspace?: AttachedWorkspaceInfo;
	selectedWorkspace?: Pick<TypesGen.Workspace, "id" | "name">;
	folder?: string;
	canConfigureAgentSetup: boolean;
	providerCount?: number;
	modelCount?: number;
	unsupportedProviderNames: readonly string[];
	aiGatewayDisabled?: boolean;
};

/** Displays another owner's chat metadata without mounting an editor or its actions. */
export const ReadOnlyChatComposer: React.FC<ReadOnlyChatComposerProps> = ({
	chat,
	model,
	modelSelectorPlaceholder,
	isModelCatalogLoading,
	contextUsage,
	onRefreshContext,
	isRefreshingContext,
	queuedMessages,
	queuedMessageUnderEditID,
	isChatPaused,
	mcpServers,
	selectedMCPServerIds,
	workspace,
	workspaceAgent,
	attachedWorkspace,
	selectedWorkspace,
	folder,
	canConfigureAgentSetup,
	providerCount,
	modelCount,
	unsupportedProviderNames,
	aiGatewayDisabled,
}) => {
	const [chatFullWidth] = useChatFullWidth();
	const [composerElement, setComposerElement] = useState<HTMLDivElement | null>(
		null,
	);
	useChatDropdownPosition(composerElement);
	const { organizations } = useDashboard();
	const chatOrganization = organizations.find(
		(organization) => organization.id === chat.organization_id,
	);
	const showAgentSetupNotice = shouldShowAgentSetupNotice({
		isAdmin: canConfigureAgentSetup,
		providerCount,
		modelCount,
		aiGatewayDisabled,
	});
	const chatOwnerUsername = chat.owner_username?.trim();
	const chatOwnerLabel =
		chat.owner_name?.trim() ||
		(chatOwnerUsername ? `@${chatOwnerUsername}` : "another user");
	const enabledMcpServers = mcpServers.filter((server) => server.enabled);
	const activeMcpServers = getActiveMCPServers(
		enabledMcpServers,
		selectedMCPServerIds,
	);

	return (
		<div
			className={cn(
				"mx-auto w-full pb-0 sm:pb-4",
				chatWidthClass(chatFullWidth),
			)}
		>
			{queuedMessages.length > 0 && (
				<QueuedMessagesList
					messages={queuedMessages}
					isChatPaused={isChatPaused}
					queuedMessageUnderEditID={queuedMessageUnderEditID}
					composerQueuedMessageID={null}
					className="mb-2"
				/>
			)}
			{showAgentSetupNotice && (
				<div className="relative z-0 -mb-10">
					<AgentSetupNotice
						isAdmin={canConfigureAgentSetup}
						providerCount={canConfigureAgentSetup ? (providerCount ?? 0) : 0}
						modelCount={canConfigureAgentSetup ? (modelCount ?? 0) : 0}
						organization={chatOrganization}
						unsupportedProviderNames={unsupportedProviderNames}
						aiGatewayDisabled={aiGatewayDisabled}
					/>
				</div>
			)}
			<div
				ref={setComposerElement}
				data-testid="chat-composer"
				className={cn(
					"relative z-10 rounded-2xl border bg-surface-secondary sm:bg-surface-secondary/45 p-1 shadow-xs",
					showAgentSetupNotice && "sm:bg-surface-secondary",
				)}
			>
				<div
					role="status"
					className="flex items-center gap-2 px-3 py-2 text-[13px] text-content-primary"
				>
					<InfoIcon className="size-4 shrink-0 text-content-link" />
					This chat is owned by {chatOwnerLabel}. It is read-only.
				</div>
				<div className="flex items-center justify-between gap-2 px-2.5 pb-1.5">
					<div className="flex min-w-0 flex-1 items-center gap-1">
						{isModelCatalogLoading ? (
							<Skeleton className="h-6 w-24 rounded" />
						) : (
							<span className="inline-flex min-w-0 shrink items-center gap-1 rounded-full bg-surface-secondary px-2 py-0.5 text-xs text-content-secondary">
								{model && (
									<ProviderIcon
										provider={model.provider}
										icon={model.providerIcon}
										className="size-3 shrink-0"
									/>
								)}
								<span className="truncate">
									{model?.displayName ?? modelSelectorPlaceholder}
								</span>
							</span>
						)}
						<ChatToolBadges
							planModeEnabled={chat.plan_mode === "plan"}
							collapsePlanningBadge={contextUsage !== undefined}
							workspace={workspace}
							workspaceAgent={workspaceAgent}
							chatId={chat.id}
							folder={folder}
							attachedWorkspace={attachedWorkspace}
							selectedWorkspace={selectedWorkspace}
							mcpServers={
								activeMcpServers.length > 0 ? activeMcpServers : undefined
							}
							mcpMenu={
								<MCPServersMenuContent
									servers={enabledMcpServers}
									selectedServerIds={selectedMCPServerIds}
									isDisabled
								/>
							}
							isDisabled
						/>
					</div>
					{contextUsage !== undefined && (
						<div className="flex shrink-0 items-center">
							<ContextUsageIndicator
								usage={contextUsage}
								onRefreshContext={onRefreshContext}
								isRefreshingContext={isRefreshingContext}
							/>
						</div>
					)}
				</div>
			</div>
		</div>
	);
};
