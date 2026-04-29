import { ArchiveIcon } from "lucide-react";

import {
	type FC,
	type ReactNode,
	type RefObject,
	useRef,
	useState,
} from "react";
import { useQueryClient } from "react-query";
import type { UrlTransform } from "streamdown";
import { chatDiffContentsKey } from "#/api/queries/chats";
import type * as TypesGen from "#/api/typesGenerated";
import type { ChatDiffStatus, ChatMessagePart } from "#/api/typesGenerated";
import { cn } from "#/utils/cn";
import { pageTitle } from "#/utils/page";
import {
	getPersistedSidebarTabId,
	savePersistedSidebarTabId,
} from "./AgentChatPage";
import {
	AgentChatInput,
	type ChatMessageInputRef,
} from "./components/AgentChatInput";
import {
	ChatConversationSkeleton,
	RightPanelSkeleton,
} from "./components/AgentsSkeletons";
import type { useChatStore } from "./components/ChatConversation/chatStore";
import type { ModelSelectorOption } from "./components/ChatElements";
import { DesktopPanelContext } from "./components/ChatElements/tools/DesktopPanelContext";
import type { PendingAttachment } from "./components/ChatPageContent";
import { ChatPageInput, ChatPageTimeline } from "./components/ChatPageContent";
import { ChatScrollContainer } from "./components/ChatScrollContainer";
import { ChatTopBar } from "./components/ChatTopBar";
import { GitPanel } from "./components/GitPanel/GitPanel";
import { DebugPanel } from "./components/RightPanel/DebugPanel/DebugPanel";
import { RightPanel } from "./components/RightPanel/RightPanel";
import { getEffectiveTabId } from "./components/Sidebar/getEffectiveTabId";
import { SidebarTabView } from "./components/Sidebar/SidebarTabView";
import { getWorkspaceStatus, StatusIcon } from "./components/StatusIcon";
import { TerminalPanel } from "./components/TerminalPanel";
import { ChatWorkspaceContext } from "./context/ChatWorkspaceContext";
import { chatWidthClass, useChatFullWidth } from "./hooks/useChatFullWidth";
import type { ChatDetailError } from "./utils/usageLimitMessage";

type ChatStoreHandle = ReturnType<typeof useChatStore>["store"];

// Re-use the inner presentational components directly. They are

interface EditingState {
	chatInputRef: RefObject<ChatMessageInputRef | null>;
	editorInitialValue: string;
	initialEditorState: string | undefined;
	remountKey: number;
	editingMessageId: number | null;
	editingFileBlocks: readonly ChatMessagePart[];
	handleEditUserMessage: (
		messageId: number,
		text: string,
		fileBlocks?: readonly ChatMessagePart[],
	) => void;
	handleCancelHistoryEdit: () => void;
	editingQueuedMessageID: number | null;
	handleStartQueueEdit: (
		id: number,
		text: string,
		fileBlocks: readonly ChatMessagePart[],
	) => void;
	handleCancelQueueEdit: () => void;
	handleSendFromInput: (
		message: string,
		attachments?: readonly PendingAttachment[],
	) => void;
	handleContentChange: (
		content: string,
		serializedEditorState: string,
		hasFileReferences: boolean,
	) => void;
}

interface AgentChatPageViewProps {
	// Chat data.
	agentId: string;
	organizationId: string | undefined;
	chatTitle: string | undefined;
	parentChat: TypesGen.Chat | undefined;
	persistedError: ChatDetailError | undefined;
	isArchived: boolean;
	workspaceAgent?: TypesGen.WorkspaceAgent;
	workspace?: TypesGen.Workspace;
	chatBuildId?: string;

	// Store handle.
	store: ChatStoreHandle;

	// Editing state.
	editing: EditingState;

	// Model/input configuration.
	effectiveSelectedModel: string;
	setSelectedModel: (model: string) => void;
	modelOptions: readonly ModelSelectorOption[];
	modelSelectorPlaceholder: string;
	modelSelectorHelp?: ReactNode;
	hasModelOptions: boolean;
	isModelCatalogLoading?: boolean;
	planModeEnabled?: boolean;
	onPlanModeToggle?: (enabled: boolean) => void;
	compressionThreshold: number | undefined;
	isInputDisabled: boolean;
	isSubmissionPending: boolean;
	isInterruptPending: boolean;
	workspaceOptions?: readonly TypesGen.Workspace[];
	selectedWorkspaceId?: string | null;
	onWorkspaceChange?: (workspaceId: string | null) => void;
	isWorkspaceLoading?: boolean;

	// Sidebar / panel state.
	isSidebarCollapsed: boolean;
	onToggleSidebarCollapsed: () => void;

	// Right panel state (owned by the parent so loading and
	// loaded views share the same layout).
	showSidebarPanel: boolean;
	onSetShowSidebarPanel: (next: boolean | ((prev: boolean) => boolean)) => void;

	// Sidebar content data.
	prNumber: number | undefined;
	diffStatusData: ChatDiffStatus | undefined;
	debugLoggingEnabled: boolean;
	gitWatcher: {
		repositories: ReadonlyMap<string, TypesGen.WorkspaceAgentRepoChanges>;
		everDirty: ReadonlySet<string>;
		refresh: () => boolean;
	};

	// Workspace action handlers.
	sshCommand: string | undefined;
	handleCommit: (repoRoot: string) => void;

	// Chat action handlers.
	handleInterrupt: () => void;
	handleDeleteQueuedMessage: (id: number) => Promise<void>;
	handlePromoteQueuedMessage: (id: number) => Promise<void>;

	onImplementPlan?: () => Promise<void> | void;
	onSendAskUserQuestionResponse?: (message: string) => Promise<void> | void;

	// Archive actions.
	handleArchiveAgentAction: () => void;
	handleUnarchiveAgentAction: () => void;
	handleArchiveAndDeleteWorkspaceAction: () => void;
	handleRegenerateTitle?: () => void;
	isRegeneratingTitle?: boolean;
	isRegenerateTitleDisabled?: boolean;

	// Scroll container ref.
	scrollContainerRef: RefObject<HTMLDivElement | null>;
	scrollToBottomRef?: RefObject<(() => void) | null>;

	// Pagination for loading older messages.
	hasMoreMessages: boolean;
	isFetchingMoreMessages: boolean;
	onFetchMoreMessages: () => void;
	messageCount: number;

	urlTransform?: UrlTransform;

	// MCP server state.
	mcpServers: readonly TypesGen.MCPServerConfig[];
	selectedMCPServerIds: readonly string[];
	onMCPSelectionChange: (ids: string[]) => void;
	onMCPAuthComplete: (serverId: string) => void;

	// Desktop chat ID (optional).
	desktopChatId?: string;

	lastInjectedContext?: readonly TypesGen.ChatMessagePart[];
}

export const AgentChatPageView: FC<AgentChatPageViewProps> = ({
	agentId,
	organizationId,
	chatTitle,
	parentChat,
	persistedError,
	isArchived,
	workspaceAgent,
	workspace,
	chatBuildId,
	store,
	editing,
	effectiveSelectedModel,
	setSelectedModel,
	modelOptions,
	modelSelectorPlaceholder,
	modelSelectorHelp,
	hasModelOptions,
	isModelCatalogLoading = false,
	planModeEnabled,
	onPlanModeToggle,
	compressionThreshold,
	isInputDisabled,
	isSubmissionPending,
	isInterruptPending,
	workspaceOptions = [],
	selectedWorkspaceId = null,
	onWorkspaceChange = () => {},
	isWorkspaceLoading = false,
	isSidebarCollapsed,
	onToggleSidebarCollapsed,
	showSidebarPanel,
	onSetShowSidebarPanel,
	prNumber,
	diffStatusData,
	debugLoggingEnabled,
	gitWatcher,
	sshCommand,
	handleCommit,
	handleInterrupt,
	handleDeleteQueuedMessage,
	handlePromoteQueuedMessage,
	onImplementPlan,
	onSendAskUserQuestionResponse,
	handleArchiveAgentAction,
	handleUnarchiveAgentAction,
	handleArchiveAndDeleteWorkspaceAction,
	handleRegenerateTitle,
	isRegeneratingTitle,
	isRegenerateTitleDisabled,
	scrollContainerRef,
	scrollToBottomRef,
	hasMoreMessages,
	isFetchingMoreMessages,
	onFetchMoreMessages,
	messageCount,
	urlTransform,
	mcpServers,
	selectedMCPServerIds,
	onMCPSelectionChange,
	onMCPAuthComplete,
	desktopChatId,
	lastInjectedContext,
}) => {
	const queryClient = useQueryClient();

	// Wrap the git watcher refresh to also invalidate the cached
	// remote/PR diff contents so the panel re-fetches from GitHub.
	const canSendAskUserQuestionResponse =
		!isInputDisabled && !isSubmissionPending
			? onSendAskUserQuestionResponse
			: undefined;

	const handleRefresh = () => {
		const sent = gitWatcher.refresh();
		if (sent && agentId) {
			void queryClient.invalidateQueries({
				queryKey: chatDiffContentsKey(agentId),
				exact: true,
			});
		}
		return sent;
	};

	const [isRightPanelExpanded, setIsRightPanelExpanded] = useState(false);
	const [dragVisualExpanded, setDragVisualExpanded] = useState<boolean | null>(
		null,
	);
	const visualExpanded = dragVisualExpanded ?? isRightPanelExpanded;
	const internalScrollToBottomRef = useRef<(() => void) | null>(null);
	const effectiveScrollToBottomRef =
		scrollToBottomRef ?? internalScrollToBottomRef;

	const [sidebarTabId, setSidebarTabIdState] = useState<string | null>(() =>
		getPersistedSidebarTabId(agentId),
	);

	const setSidebarTabId = (tabId: string) => {
		setSidebarTabIdState(tabId);
		if (!isArchived) {
			savePersistedSidebarTabId(agentId, tabId);
		}
	};

	const handleOpenDesktop = () => {
		onSetShowSidebarPanel(true);
		setSidebarTabId("desktop");
	};

	const desktopPanelCtx = {
		desktopChatId,
		onOpenDesktop: desktopChatId ? handleOpenDesktop : undefined,
	};

	const shouldShowSidebar = showSidebarPanel;

	// Compute local diff stats from git watcher unified diffs.

	// Prefer the git repository root over the agent's expanded directory
	// for VS Code folder resolution (important for monorepos).
	const preferredFolder = (() => {
		const repoRoots = Array.from(gitWatcher?.repositories.keys() ?? []).sort();
		return repoRoots[0] || workspaceAgent?.expanded_directory;
	})();

	const workspaceRoute = workspace
		? `/@${workspace.owner_name}/${workspace.name}`
		: undefined;

	const attachedWorkspace = (() => {
		if (!workspace || !workspaceRoute) return undefined;

		const { effectiveType, statusLabel } = getWorkspaceStatus(
			workspace,
			workspaceAgent,
		);
		const statusIcon = <StatusIcon type={effectiveType} />;
		return {
			id: workspace.id,
			name: workspace.name,
			route: workspaceRoute,
			statusIcon,
			statusLabel,
		};
	})();

	// Desktop is only available when the workspace + agent are ready;
	// `SidebarTabView` gates the desktop tab/panel on the same condition,
	// so resolve tab selection against the same availability to avoid
	// picking "desktop" when no desktop panel is rendered.
	const availableDesktopChatId =
		workspace && workspaceAgent ? desktopChatId : undefined;
	// Single source of truth for available tabs and their order. The list
	// of tab IDs used by `getEffectiveTabId` is derived from this so a
	// new tab can never be added to one without the other going out of
	// sync.
	const sidebarTabConfigs = [
		{ id: "git", label: "Git" },
		...(workspace && workspaceAgent
			? [{ id: "terminal", label: "Terminal" }]
			: []),
		...(debugLoggingEnabled ? [{ id: "debug", label: "Debug" }] : []),
	];
	const sidebarTabIds = sidebarTabConfigs.map((tab) => tab.id);
	const effectiveSidebarTabId = getEffectiveTabId(
		sidebarTabIds,
		sidebarTabId,
		availableDesktopChatId,
	);
	const renderTabContent = (tabId: string): ReactNode => {
		switch (tabId) {
			case "git":
				return (
					<GitPanel
						prTab={
							prNumber && agentId ? { prNumber, chatId: agentId } : undefined
						}
						repositories={gitWatcher.repositories}
						everDirty={gitWatcher.everDirty}
						onRefresh={handleRefresh}
						onCommit={handleCommit}
						isExpanded={visualExpanded}
						remoteDiffStats={diffStatusData}
						chatInputRef={editing.chatInputRef}
					/>
				);
			case "terminal":
				return workspace && workspaceAgent ? (
					<TerminalPanel
						chatId={agentId}
						isVisible={
							shouldShowSidebar && effectiveSidebarTabId === "terminal"
						}
						workspace={workspace}
						workspaceAgent={workspaceAgent}
					/>
				) : null;
			case "debug":
				return (
					<DebugPanel
						chatId={agentId}
						isVisible={shouldShowSidebar && effectiveSidebarTabId === "debug"}
					/>
				);
			default:
				return null;
		}
	};
	const sidebarTabs = sidebarTabConfigs.map((tab) => ({
		id: tab.id,
		label: tab.label,
		content: renderTabContent(tab.id),
	}));

	const isEditing =
		editing.editingMessageId !== null ||
		editing.editingQueuedMessageID !== null;

	const titleElement = (
		<title>
			{chatTitle ? pageTitle(chatTitle, "Agents") : pageTitle("Agents")}
		</title>
	);

	return (
		<ChatWorkspaceContext
			value={{ workspaceId: workspace?.id, buildId: chatBuildId }}
		>
			<DesktopPanelContext value={desktopPanelCtx}>
				<div
					className={cn(
						"relative flex min-h-0 min-w-0 flex-1",
						shouldShowSidebar && !visualExpanded && "flex-row",
					)}
				>
					{titleElement}
					<div
						className={cn(
							"relative flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden",
							visualExpanded && "hidden",
							shouldShowSidebar && "max-lg:hidden",
						)}
					>
						<div className="relative z-10 shrink-0 overflow-visible">
							{" "}
							<ChatTopBar
								chatTitle={chatTitle}
								parentChat={parentChat}
								panel={{
									showSidebarPanel,
									onToggleSidebar: () => onSetShowSidebarPanel((prev) => !prev),
								}}
								onArchiveAgent={handleArchiveAgentAction}
								onUnarchiveAgent={handleUnarchiveAgentAction}
								onArchiveAndDeleteWorkspace={
									handleArchiveAndDeleteWorkspaceAction
								}
								{...(handleRegenerateTitle
									? { onRegenerateTitle: handleRegenerateTitle }
									: {})}
								isRegeneratingTitle={isRegeneratingTitle}
								isRegenerateTitleDisabled={isRegenerateTitleDisabled}
								hasWorkspace={Boolean(workspace)}
								isArchived={isArchived}
								diffStatusData={diffStatusData}
								isSidebarCollapsed={isSidebarCollapsed}
								onToggleSidebarCollapsed={onToggleSidebarCollapsed}
							/>
							{isArchived && (
								<div className="flex shrink-0 items-center gap-2 border-b border-border-default bg-surface-secondary px-4 py-2 text-xs text-content-secondary">
									<ArchiveIcon className="h-4 w-4 shrink-0" />
									This agent has been archived and is read-only.
								</div>
							)}
							<div
								aria-hidden
								className="pointer-events-none absolute inset-x-0 top-full z-10 h-3 sm:h-6 bg-surface-primary"
								style={{
									maskImage:
										"linear-gradient(to bottom, black 0%, rgba(0,0,0,0.6) 40%, rgba(0,0,0,0.2) 70%, transparent 100%)",
									WebkitMaskImage:
										"linear-gradient(to bottom, black 0%, rgba(0,0,0,0.6) 40%, rgba(0,0,0,0.2) 70%, transparent 100%)",
								}}
							/>
						</div>
						<ChatScrollContainer
							key={agentId}
							scrollContainerRef={scrollContainerRef}
							scrollToBottomRef={effectiveScrollToBottomRef}
							isFetchingMoreMessages={isFetchingMoreMessages}
							hasMoreMessages={hasMoreMessages}
							onFetchMoreMessages={onFetchMoreMessages}
							messageCount={messageCount}
						>
							<div className="px-4">
								<ChatPageTimeline
									chatID={agentId}
									store={store}
									persistedError={persistedError}
									onEditUserMessage={editing.handleEditUserMessage}
									editingMessageId={editing.editingMessageId}
									urlTransform={urlTransform}
									mcpServers={mcpServers}
									onImplementPlan={onImplementPlan}
									onSendAskUserQuestionResponse={canSendAskUserQuestionResponse}
								/>
							</div>
						</ChatScrollContainer>
						<div className="shrink-0 overflow-y-auto px-4 pb-3 md:pb-0 [scrollbar-gutter:stable] [scrollbar-width:thin]">
							<ChatPageInput
								organizationId={organizationId}
								store={store}
								compressionThreshold={compressionThreshold}
								onSend={editing.handleSendFromInput}
								onDeleteQueuedMessage={handleDeleteQueuedMessage}
								onPromoteQueuedMessage={handlePromoteQueuedMessage}
								onInterrupt={handleInterrupt}
								isInputDisabled={isInputDisabled}
								isSendPending={isSubmissionPending}
								isInterruptPending={isInterruptPending}
								hasModelOptions={hasModelOptions}
								selectedModel={effectiveSelectedModel}
								onModelChange={setSelectedModel}
								modelOptions={modelOptions}
								modelSelectorPlaceholder={modelSelectorPlaceholder}
								modelSelectorHelp={modelSelectorHelp}
								planModeEnabled={planModeEnabled}
								onPlanModeToggle={onPlanModeToggle}
								isModelCatalogLoading={isModelCatalogLoading}
								workspaceOptions={workspaceOptions}
								selectedWorkspaceId={selectedWorkspaceId}
								onWorkspaceChange={onWorkspaceChange}
								isWorkspaceLoading={isWorkspaceLoading}
								inputRef={editing.chatInputRef}
								initialValue={editing.editorInitialValue}
								initialEditorState={editing.initialEditorState}
								remountKey={editing.remountKey}
								onContentChange={editing.handleContentChange}
								isEditing={isEditing}
								editingQueuedMessageID={editing.editingQueuedMessageID}
								onStartQueueEdit={editing.handleStartQueueEdit}
								onCancelQueueEdit={editing.handleCancelQueueEdit}
								isEditingHistoryMessage={editing.editingMessageId !== null}
								onCancelHistoryEdit={editing.handleCancelHistoryEdit}
								onEditUserMessage={editing.handleEditUserMessage}
								editingFileBlocks={editing.editingFileBlocks}
								mcpServers={mcpServers}
								selectedMCPServerIds={selectedMCPServerIds}
								onMCPSelectionChange={onMCPSelectionChange}
								onMCPAuthComplete={onMCPAuthComplete}
								lastInjectedContext={lastInjectedContext}
								workspace={workspace}
								workspaceAgent={workspaceAgent}
								chatId={agentId}
								sshCommand={sshCommand}
								attachedWorkspace={attachedWorkspace}
								folder={preferredFolder}
							/>
						</div>
					</div>
					<RightPanel
						isOpen={shouldShowSidebar}
						isExpanded={isRightPanelExpanded}
						onToggleExpanded={() => setIsRightPanelExpanded((prev) => !prev)}
						onClose={() => onSetShowSidebarPanel(false)}
						onVisualExpandedChange={setDragVisualExpanded}
						isSidebarCollapsed={isSidebarCollapsed}
						onToggleSidebarCollapsed={onToggleSidebarCollapsed}
					>
						<SidebarTabView
							effectiveTabId={effectiveSidebarTabId}
							onActiveTabChange={setSidebarTabId}
							tabs={sidebarTabs}
							onClose={() => onSetShowSidebarPanel(false)}
							isExpanded={visualExpanded}
							onToggleExpanded={() => setIsRightPanelExpanded((prev) => !prev)}
							isSidebarCollapsed={isSidebarCollapsed}
							onToggleSidebarCollapsed={onToggleSidebarCollapsed}
							chatTitle={chatTitle}
							desktopChatId={availableDesktopChatId}
						/>
					</RightPanel>
				</div>
			</DesktopPanelContext>
		</ChatWorkspaceContext>
	);
};

interface AgentChatPageLoadingViewProps {
	titleElement: React.ReactNode;
	isInputDisabled: boolean;
	effectiveSelectedModel: string;
	setSelectedModel: (model: string) => void;
	modelOptions: readonly ModelSelectorOption[];
	modelSelectorPlaceholder: string;
	hasModelOptions: boolean;
	isModelCatalogLoading?: boolean;
	planModeEnabled?: boolean;
	onPlanModeToggle?: (enabled: boolean) => void;
	isSidebarCollapsed: boolean;
	onToggleSidebarCollapsed: () => void;
	showRightPanel: boolean;
}

export const AgentChatPageLoadingView: FC<AgentChatPageLoadingViewProps> = ({
	titleElement,
	isInputDisabled,
	effectiveSelectedModel,
	setSelectedModel,
	modelOptions,
	modelSelectorPlaceholder,
	hasModelOptions,
	isModelCatalogLoading = false,
	planModeEnabled,
	onPlanModeToggle,
	isSidebarCollapsed,
	onToggleSidebarCollapsed,
	showRightPanel,
}) => {
	const [chatFullWidth] = useChatFullWidth();
	return (
		<div
			className={cn(
				"relative flex h-full min-h-0 min-w-0 flex-1",
				showRightPanel && "flex-row",
			)}
		>
			{titleElement}
			<div className="relative flex h-full min-h-0 min-w-0 flex-1 flex-col">
				<ChatTopBar
					panel={{
						showSidebarPanel: false,
						onToggleSidebar: () => {},
					}}
					onArchiveAgent={() => {}}
					onUnarchiveAgent={() => {}}
					onRegenerateTitle={() => {}}
					onArchiveAndDeleteWorkspace={() => {}}
					hasWorkspace={false}
					isSidebarCollapsed={isSidebarCollapsed}
					onToggleSidebarCollapsed={onToggleSidebarCollapsed}
				/>
				<div className="min-h-0 flex-1 overflow-y-auto [scrollbar-gutter:stable] [scrollbar-width:thin] [scrollbar-color:hsl(var(--surface-quaternary))_transparent]">
					<div className="px-4">
						<div
							className={cn(
								"mx-auto w-full py-6",
								chatWidthClass(chatFullWidth),
							)}
						>
							<ChatConversationSkeleton />
						</div>
					</div>
				</div>
				<div className="shrink-0 overflow-y-auto px-4 pb-3 md:pb-0 [scrollbar-gutter:stable] [scrollbar-width:thin]">
					<AgentChatInput
						onSend={() => {}}
						initialValue=""
						isDisabled={isInputDisabled}
						isLoading={false}
						selectedModel={effectiveSelectedModel}
						onModelChange={setSelectedModel}
						modelOptions={modelOptions}
						modelSelectorPlaceholder={modelSelectorPlaceholder}
						planModeEnabled={planModeEnabled}
						onPlanModeToggle={onPlanModeToggle}
						isModelCatalogLoading={isModelCatalogLoading}
						hasModelOptions={hasModelOptions}
					/>
				</div>{" "}
			</div>
			{showRightPanel && (
				<RightPanel
					isOpen
					isExpanded={false}
					onToggleExpanded={() => {}}
					onClose={() => {}}
					isSidebarCollapsed={isSidebarCollapsed}
					onToggleSidebarCollapsed={onToggleSidebarCollapsed}
				>
					<RightPanelSkeleton />
				</RightPanel>
			)}
		</div>
	);
};

interface AgentChatPageNotFoundViewProps {
	titleElement: React.ReactNode;
	isSidebarCollapsed: boolean;
	onToggleSidebarCollapsed: () => void;
}

export const AgentChatPageNotFoundView: FC<AgentChatPageNotFoundViewProps> = ({
	titleElement,
	isSidebarCollapsed,
	onToggleSidebarCollapsed,
}) => {
	return (
		<div className="flex h-full min-h-0 min-w-0 flex-1 flex-col">
			{titleElement}
			<ChatTopBar
				panel={{
					showSidebarPanel: false,
					onToggleSidebar: () => {},
				}}
				onArchiveAgent={() => {}}
				onUnarchiveAgent={() => {}}
				onRegenerateTitle={() => {}}
				onArchiveAndDeleteWorkspace={() => {}}
				hasWorkspace={false}
				isSidebarCollapsed={isSidebarCollapsed}
				onToggleSidebarCollapsed={onToggleSidebarCollapsed}
			/>
			<div className="flex flex-1 items-center justify-center text-content-secondary">
				Chat not found
			</div>
		</div>
	);
};
