import { ArchiveIcon } from "lucide-react";
import { type FC, type RefObject, useRef, useState } from "react";
import type { UrlTransform } from "streamdown";
import type * as TypesGen from "#/api/typesGenerated";
import type { ChatDiffStatus, ChatMessagePart } from "#/api/typesGenerated";

import { cn } from "#/utils/cn";
import { pageTitle } from "#/utils/page";
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
import { ChatPageInput, ChatPageTimeline } from "./components/ChatPageContent";
import { ChatScrollContainer } from "./components/ChatScrollContainer";
import { ChatTopBar } from "./components/ChatTopBar";
import { GitPanel } from "./components/GitPanel/GitPanel";
import { RightPanel } from "./components/RightPanel/RightPanel";
import { SidebarTabView } from "./components/Sidebar/SidebarTabView";
import { TerminalPanel } from "./components/TerminalPanel";
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
	handleSendFromInput: (message: string, fileIds?: string[]) => void;
	handleContentChange: (
		content: string,
		serializedEditorState: string,
		hasFileReferences: boolean,
	) => void;
}

interface AgentChatPageViewProps {
	// Chat data.
	agentId: string;
	chatTitle: string | undefined;
	parentChat: TypesGen.Chat | undefined;
	persistedError: ChatDetailError | undefined;
	isArchived: boolean;
	hasWorkspace: boolean;
	workspaceAgent?: TypesGen.WorkspaceAgent;
	workspace?: TypesGen.Workspace;

	// Store handle.
	store: ChatStoreHandle;

	// Editing state.
	editing: EditingState;
	pendingEditMessageId: number | null;

	// Model/input configuration.
	effectiveSelectedModel: string;
	setSelectedModel: (model: string) => void;
	modelOptions: readonly ModelSelectorOption[];
	modelSelectorPlaceholder: string;
	hasModelOptions: boolean;
	isModelCatalogLoading?: boolean;
	compressionThreshold: number | undefined;
	isInputDisabled: boolean;
	isSubmissionPending: boolean;
	isInterruptPending: boolean;

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
	gitWatcher: {
		repositories: ReadonlyMap<string, TypesGen.WorkspaceAgentRepoChanges>;
		refresh: () => boolean;
	};

	// Workspace action handlers.
	canOpenEditors: boolean;
	canOpenWorkspace: boolean;
	sshCommand: string | undefined;
	handleOpenInEditor: (editor: "cursor" | "vscode") => void;
	handleViewWorkspace: () => void;
	handleOpenTerminal: () => void;
	handleCommit: (repoRoot: string) => void;

	// Chat action handlers.
	handleInterrupt: () => void;
	handleDeleteQueuedMessage: (id: number) => Promise<void>;
	handlePromoteQueuedMessage: (id: number) => Promise<void>;

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
	chatTitle,
	parentChat,
	persistedError,
	isArchived,
	hasWorkspace,
	workspaceAgent,
	workspace,
	store,
	editing,
	pendingEditMessageId,
	effectiveSelectedModel,
	setSelectedModel,
	modelOptions,
	modelSelectorPlaceholder,
	hasModelOptions,
	isModelCatalogLoading = false,
	compressionThreshold,
	isInputDisabled,
	isSubmissionPending,
	isInterruptPending,
	isSidebarCollapsed,
	onToggleSidebarCollapsed,
	showSidebarPanel,
	onSetShowSidebarPanel,
	prNumber,
	diffStatusData,
	gitWatcher,
	canOpenEditors,
	canOpenWorkspace,
	sshCommand,
	handleOpenInEditor,
	handleViewWorkspace,
	handleOpenTerminal,
	handleCommit,
	handleInterrupt,
	handleDeleteQueuedMessage,
	handlePromoteQueuedMessage,
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
	urlTransform,
	mcpServers,
	selectedMCPServerIds,
	onMCPSelectionChange,
	onMCPAuthComplete,
	desktopChatId,
	lastInjectedContext,
}) => {
	const [isRightPanelExpanded, setIsRightPanelExpanded] = useState(false);
	const [dragVisualExpanded, setDragVisualExpanded] = useState<boolean | null>(
		null,
	);
	const visualExpanded = dragVisualExpanded ?? isRightPanelExpanded;
	const internalScrollToBottomRef = useRef<(() => void) | null>(null);
	const effectiveScrollToBottomRef =
		scrollToBottomRef ?? internalScrollToBottomRef;

	// State for programmatically switching the sidebar tab (e.g. when
	// the user clicks the inline desktop preview card).
	const [sidebarTabId, setSidebarTabId] = useState<string | null>(null);

	const handleOpenDesktop = () => {
		onSetShowSidebarPanel(true);
		setSidebarTabId("desktop");
	};

	const desktopPanelCtx = {
		desktopChatId,
		onOpenDesktop: desktopChatId ? handleOpenDesktop : undefined,
	};

	// Compute local diff stats from git watcher unified diffs.

	const titleElement = (
		<title>
			{chatTitle ? pageTitle(chatTitle, "Agents") : pageTitle("Agents")}
		</title>
	);

	const shouldShowSidebar = showSidebarPanel;

	return (
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
						shouldShowSidebar && "max-md:hidden",
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
							workspace={{
								canOpenEditors,
								canOpenWorkspace,
								onOpenInEditor: handleOpenInEditor,
								onViewWorkspace: handleViewWorkspace,
								onOpenTerminal: handleOpenTerminal,
								sshCommand,
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
							hasWorkspace={hasWorkspace}
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
					>
						<div className="px-4">
							<ChatPageTimeline
								chatID={agentId}
								store={store}
								persistedError={persistedError}
								onEditUserMessage={editing.handleEditUserMessage}
								editingMessageId={editing.editingMessageId}
								savingMessageId={pendingEditMessageId}
								urlTransform={urlTransform}
								mcpServers={mcpServers}
							/>
						</div>
					</ChatScrollContainer>
					<div className="shrink-0 overflow-y-auto px-4 pb-4 md:pb-0 [scrollbar-gutter:stable] [scrollbar-width:thin]">
						<ChatPageInput
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
							isModelCatalogLoading={isModelCatalogLoading}
							inputRef={editing.chatInputRef}
							initialValue={editing.editorInitialValue}
							initialEditorState={editing.initialEditorState}
							remountKey={editing.remountKey}
							onContentChange={editing.handleContentChange}
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
						activeTabId={sidebarTabId}
						onActiveTabChange={setSidebarTabId}
						tabs={[
							{
								id: "git",
								label: "Git",
								content: (
									<GitPanel
										prTab={
											prNumber && agentId
												? { prNumber, chatId: agentId }
												: undefined
										}
										repositories={gitWatcher.repositories}
										onRefresh={gitWatcher.refresh}
										onCommit={handleCommit}
										isExpanded={visualExpanded}
										remoteDiffStats={diffStatusData}
										chatInputRef={editing.chatInputRef}
									/>
								),
							},
							...(hasWorkspace && workspaceAgent
								? [
										{
											id: "terminal",
											label: "Terminal",
											content: (
												<TerminalPanel
													chatId={agentId}
													isVisible={
														shouldShowSidebar && sidebarTabId === "terminal"
													}
													workspace={workspace}
													workspaceAgent={workspaceAgent}
												/>
											),
										},
									]
								: []),
						]}
						onClose={() => onSetShowSidebarPanel(false)}
						isExpanded={visualExpanded}
						onToggleExpanded={() => setIsRightPanelExpanded((prev) => !prev)}
						isSidebarCollapsed={isSidebarCollapsed}
						onToggleSidebarCollapsed={onToggleSidebarCollapsed}
						chatTitle={chatTitle}
						desktopChatId={desktopChatId}
					/>
				</RightPanel>
			</div>
		</DesktopPanelContext>
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
	isSidebarCollapsed,
	onToggleSidebarCollapsed,
	showRightPanel,
}) => {
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
					workspace={{
						canOpenEditors: false,
						canOpenWorkspace: false,
						onOpenInEditor: () => {},
						onViewWorkspace: () => {},
						onOpenTerminal: () => {},
						sshCommand: undefined,
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
						<div className="mx-auto w-full max-w-3xl py-6">
							<ChatConversationSkeleton />
						</div>
					</div>
				</div>
				<div className="shrink-0 overflow-y-auto px-4 pb-4 md:pb-0 [scrollbar-gutter:stable] [scrollbar-width:thin]">
					<AgentChatInput
						onSend={() => {}}
						initialValue=""
						isDisabled={isInputDisabled}
						isLoading={false}
						selectedModel={effectiveSelectedModel}
						onModelChange={setSelectedModel}
						modelOptions={modelOptions}
						modelSelectorPlaceholder={modelSelectorPlaceholder}
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
				workspace={{
					canOpenEditors: false,
					canOpenWorkspace: false,
					onOpenInEditor: () => {},
					onViewWorkspace: () => {},
					onOpenTerminal: () => {},
					sshCommand: undefined,
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
