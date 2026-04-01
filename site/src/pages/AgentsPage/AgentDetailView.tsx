import type { ChatDiffStatusResponse } from "api/api";
import type * as TypesGen from "api/typesGenerated";
import type { ModelSelectorOption } from "components/ai-elements";
import { Skeleton } from "components/Skeleton/Skeleton";
import { ArchiveIcon } from "lucide-react";
import { type FC, type RefObject, useCallback, useMemo, useState } from "react";
import type { UrlTransform } from "streamdown";
import { cn } from "utils/cn";
import { pageTitle } from "utils/page";
import { AgentChatInput, type ChatMessageInputRef } from "./AgentChatInput";
import { AgentDetailInput, AgentDetailTimeline } from "./AgentDetail";
import type { useChatStore } from "./AgentDetail/ChatContext";
import { AgentDetailTopBar } from "./AgentDetail/TopBar";
import { GitPanel } from "./GitPanel";
import { RightPanel } from "./RightPanel";
import { SidebarTabView } from "./SidebarTabView";

type ChatStoreHandle = ReturnType<typeof useChatStore>["store"];

// Re-use the inner presentational components directly. They are

interface EditingState {
	chatInputRef: RefObject<ChatMessageInputRef | null>;
	editorInitialValue: string;
	editingMessageId: number | null;
	editingFileBlocks: readonly {
		mediaType: string;
		data?: string;
		fileId?: string;
	}[];
	handleEditUserMessage: (
		messageId: number,
		text: string,
		fileBlocks?: readonly {
			mediaType: string;
			data?: string;
			fileId?: string;
		}[],
	) => void;
	handleCancelHistoryEdit: () => void;
	editingQueuedMessageID: number | null;
	handleStartQueueEdit: (id: number, text: string) => void;
	handleCancelQueueEdit: () => void;
	handleSendFromInput: (message: string, fileIds?: string[]) => void;
	handleContentChange: (content: string) => void;
}

interface AgentDetailViewProps {
	// Chat data.
	agentId: string;
	chatTitle: string | undefined;
	parentChat: TypesGen.Chat | undefined;
	chatErrorReasons: Record<string, string>;
	chatRecord: TypesGen.Chat | undefined;
	isArchived: boolean;
	hasWorkspace: boolean;

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
	inputStatusText: string | null;
	modelCatalogStatusMessage: string | null;
	compressionThreshold: number | undefined;
	isInputDisabled: boolean;
	isSubmissionPending: boolean;
	isInterruptPending: boolean;

	// Sidebar / panel state.
	isSidebarCollapsed: boolean;
	onToggleSidebarCollapsed: () => void;

	// Sidebar content data.
	prNumber: number | undefined;
	diffStatusData: ChatDiffStatusResponse | undefined;
	gitWatcher: {
		repositories: ReadonlyMap<string, TypesGen.WorkspaceAgentRepoChanges>;
		refresh: () => void;
	};

	// Workspace action handlers.
	canOpenEditors: boolean;
	canOpenWorkspace: boolean;
	sshCommand: string | undefined;
	handleOpenInEditor: (editor: "cursor" | "vscode") => void;
	handleViewWorkspace: () => void;
	handleOpenTerminal: () => void;
	handleCommit: (repoRoot: string) => void;

	// Navigation.
	onNavigateToChat: (chatId: string) => void;

	// Chat action handlers.
	handleInterrupt: () => void;
	handleDeleteQueuedMessage: (id: number) => Promise<void>;
	handlePromoteQueuedMessage: (id: number) => Promise<void>;

	// Archive actions.
	handleArchiveAgentAction: () => void;
	handleUnarchiveAgentAction: () => void;
	handleArchiveAndDeleteWorkspaceAction: () => void;

	// Scroll container ref.
	scrollContainerRef: RefObject<HTMLDivElement | null>;

	urlTransform?: UrlTransform;
}

/** localStorage key controlling whether the right panel is visible. */
export const RIGHT_PANEL_OPEN_KEY = "agents.right-panel-open";

export const AgentDetailView: FC<AgentDetailViewProps> = ({
	agentId,
	chatTitle,
	parentChat,
	chatErrorReasons,
	chatRecord,
	isArchived,
	hasWorkspace,
	store,
	editing,
	pendingEditMessageId,
	effectiveSelectedModel,
	setSelectedModel,
	modelOptions,
	modelSelectorPlaceholder,
	hasModelOptions,
	inputStatusText,
	modelCatalogStatusMessage,
	compressionThreshold,
	isInputDisabled,
	isSubmissionPending,
	isInterruptPending,
	isSidebarCollapsed,
	onToggleSidebarCollapsed,
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
	onNavigateToChat,
	handleInterrupt,
	handleDeleteQueuedMessage,
	handlePromoteQueuedMessage,
	handleArchiveAgentAction,
	handleUnarchiveAgentAction,
	handleArchiveAndDeleteWorkspaceAction,
	scrollContainerRef,
	urlTransform,
}) => {
	// Panel/sidebar UI state – purely visual, no data-fetching
	// implications. The open/closed state is persisted to localStorage
	// so users get a consistent layout when switching between chats.
	const [showSidebarPanel, setShowSidebarPanel] = useState(() => {
		if (typeof window === "undefined") return false;
		return localStorage.getItem(RIGHT_PANEL_OPEN_KEY) === "true";
	});
	const handleSetShowSidebarPanel = useCallback(
		(next: boolean | ((prev: boolean) => boolean)) => {
			setShowSidebarPanel((prev) => {
				const value = typeof next === "function" ? next(prev) : next;
				if (typeof window !== "undefined") {
					localStorage.setItem(RIGHT_PANEL_OPEN_KEY, String(value));
				}
				return value;
			});
		},
		[],
	);
	const [isRightPanelExpanded, setIsRightPanelExpanded] = useState(false);
	const [dragVisualExpanded, setDragVisualExpanded] = useState<boolean | null>(
		null,
	);
	const visualExpanded = dragVisualExpanded ?? isRightPanelExpanded;

	// Compute local diff stats from git watcher unified diffs.
	const localDiffStats = useMemo(() => {
		let additions = 0;
		let deletions = 0;
		for (const repo of gitWatcher.repositories.values()) {
			if (!repo.unified_diff) continue;
			for (const line of repo.unified_diff.split("\n")) {
				if (line.startsWith("+") && !line.startsWith("+++")) {
					additions++;
				} else if (line.startsWith("-") && !line.startsWith("---")) {
					deletions++;
				}
			}
		}
		return { additions, deletions, changed_files: 0 };
	}, [gitWatcher.repositories]);

	const titleElement = (
		<title>
			{chatTitle ? pageTitle(chatTitle, "Agents") : pageTitle("Agents")}
		</title>
	);

	const shouldShowSidebar = showSidebarPanel;

	return (
		<div
			className={cn(
				"relative flex min-h-0 min-w-0 flex-1",
				shouldShowSidebar && !visualExpanded && "flex-row",
			)}
		>
			{titleElement}
			<div
				className={cn(
					"relative flex min-h-0 min-w-0 flex-1 flex-col",
					visualExpanded && "hidden",
					shouldShowSidebar && "max-md:hidden",
				)}
			>
				<div className="relative z-10 shrink-0 overflow-visible">
					<AgentDetailTopBar
						chatTitle={chatTitle}
						parentChat={parentChat}
						onOpenParentChat={(chatId) => onNavigateToChat(chatId)}
						panel={{
							showSidebarPanel,
							onToggleSidebar: () => handleSetShowSidebarPanel((prev) => !prev),
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
						onArchiveAndDeleteWorkspace={handleArchiveAndDeleteWorkspaceAction}
						hasWorkspace={hasWorkspace}
						isArchived={isArchived}
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
						className="pointer-events-none absolute inset-x-0 top-full z-10 h-6 bg-surface-primary"
						style={{
							maskImage:
								"linear-gradient(to bottom, black 0%, rgba(0,0,0,0.6) 40%, rgba(0,0,0,0.2) 70%, transparent 100%)",
							WebkitMaskImage:
								"linear-gradient(to bottom, black 0%, rgba(0,0,0,0.6) 40%, rgba(0,0,0,0.2) 70%, transparent 100%)",
						}}
					/>
				</div>
				<div
					ref={scrollContainerRef}
					className="flex min-h-0 flex-1 flex-col-reverse overflow-y-auto [scrollbar-gutter:stable] [scrollbar-width:thin] [scrollbar-color:hsl(var(--surface-quaternary))_transparent]"
				>
					<div className="px-4">
						<AgentDetailTimeline
							store={store}
							chatID={agentId}
							persistedErrorReason={
								chatErrorReasons[agentId] || chatRecord?.last_error || undefined
							}
							onEditUserMessage={editing.handleEditUserMessage}
							editingMessageId={editing.editingMessageId}
							savingMessageId={pendingEditMessageId}
							urlTransform={urlTransform}
						/>
					</div>
				</div>
				<div className="shrink-0 overflow-y-auto px-4 [scrollbar-gutter:stable] [scrollbar-width:thin]">
					<AgentDetailInput
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
						inputStatusText={inputStatusText}
						modelCatalogStatusMessage={modelCatalogStatusMessage}
						inputRef={editing.chatInputRef}
						initialValue={editing.editorInitialValue}
						onContentChange={editing.handleContentChange}
						editingQueuedMessageID={editing.editingQueuedMessageID}
						onStartQueueEdit={editing.handleStartQueueEdit}
						onCancelQueueEdit={editing.handleCancelQueueEdit}
						isEditingHistoryMessage={editing.editingMessageId !== null}
						onCancelHistoryEdit={editing.handleCancelHistoryEdit}
						editingFileBlocks={editing.editingFileBlocks}
					/>
				</div>
			</div>
			<RightPanel
				isOpen={shouldShowSidebar}
				isExpanded={isRightPanelExpanded}
				onToggleExpanded={() => setIsRightPanelExpanded((prev) => !prev)}
				onClose={() => handleSetShowSidebarPanel(false)}
				onVisualExpandedChange={setDragVisualExpanded}
				isSidebarCollapsed={isSidebarCollapsed}
				onToggleSidebarCollapsed={onToggleSidebarCollapsed}
			>
				<SidebarTabView
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
									localDiffStats={localDiffStats}
									chatInputRef={editing.chatInputRef}
								/>
							),
						},
					]}
					onClose={() => handleSetShowSidebarPanel(false)}
					isExpanded={visualExpanded}
					onToggleExpanded={() => setIsRightPanelExpanded((prev) => !prev)}
					isSidebarCollapsed={isSidebarCollapsed}
					onToggleSidebarCollapsed={onToggleSidebarCollapsed}
					chatTitle={chatTitle}
				/>
			</RightPanel>{" "}
		</div>
	);
};

interface AgentDetailLoadingViewProps {
	titleElement: React.ReactNode;
	isInputDisabled: boolean;
	effectiveSelectedModel: string;
	setSelectedModel: (model: string) => void;
	modelOptions: readonly ModelSelectorOption[];
	modelSelectorPlaceholder: string;
	hasModelOptions: boolean;
	inputStatusText: string | null;
	modelCatalogStatusMessage: string | null;
	isSidebarCollapsed: boolean;
	onToggleSidebarCollapsed: () => void;
}

export const AgentDetailLoadingView: FC<AgentDetailLoadingViewProps> = ({
	titleElement,
	isInputDisabled,
	effectiveSelectedModel,
	setSelectedModel,
	modelOptions,
	modelSelectorPlaceholder,
	hasModelOptions,
	inputStatusText,
	modelCatalogStatusMessage,
	isSidebarCollapsed,
	onToggleSidebarCollapsed,
}) => {
	return (
		<div className="relative flex h-full min-h-0 min-w-0 flex-1 flex-col">
			{titleElement}
			<AgentDetailTopBar
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
				onOpenParentChat={() => {}}
				onArchiveAgent={() => {}}
				onUnarchiveAgent={() => {}}
				onArchiveAndDeleteWorkspace={() => {}}
				hasWorkspace={false}
				isSidebarCollapsed={isSidebarCollapsed}
				onToggleSidebarCollapsed={onToggleSidebarCollapsed}
			/>
			<div className="flex min-h-0 flex-1 flex-col-reverse overflow-hidden">
				<div className="px-4">
					<div className="mx-auto w-full max-w-3xl py-6">
						<div className="flex flex-col gap-3">
							{/* User message bubble (right-aligned) */}
							<div className="flex w-full justify-end">
								<Skeleton className="h-10 w-2/3 rounded-lg" />
							</div>
							{/* Assistant response lines (left-aligned) */}
							<div className="space-y-3">
								<Skeleton className="h-4 w-full" />
								<Skeleton className="h-4 w-5/6" />
								<Skeleton className="h-4 w-4/6" />
							</div>
							{/* Second user message bubble */}
							<div className="mt-3 flex w-full justify-end">
								<Skeleton className="h-10 w-1/2 rounded-lg" />
							</div>
							{/* Second assistant response */}
							<div className="space-y-3">
								<Skeleton className="h-4 w-full" />
								<Skeleton className="h-4 w-5/6" />
								<Skeleton className="h-4 w-4/6" />
								<Skeleton className="h-4 w-full" />
								<Skeleton className="h-4 w-3/5" />
							</div>{" "}
						</div>
					</div>
				</div>
			</div>
			<div className="shrink-0 px-4">
				<AgentChatInput
					onSend={() => {}}
					initialValue=""
					isDisabled={isInputDisabled}
					isLoading={false}
					selectedModel={effectiveSelectedModel}
					onModelChange={setSelectedModel}
					modelOptions={modelOptions}
					modelSelectorPlaceholder={modelSelectorPlaceholder}
					hasModelOptions={hasModelOptions}
					inputStatusText={inputStatusText}
					modelCatalogStatusMessage={modelCatalogStatusMessage}
				/>
			</div>
		</div>
	);
};

interface AgentDetailNotFoundViewProps {
	titleElement: React.ReactNode;
	isSidebarCollapsed: boolean;
	onToggleSidebarCollapsed: () => void;
}

export const AgentDetailNotFoundView: FC<AgentDetailNotFoundViewProps> = ({
	titleElement,
	isSidebarCollapsed,
	onToggleSidebarCollapsed,
}) => {
	return (
		<div className="flex h-full min-h-0 min-w-0 flex-1 flex-col">
			{titleElement}
			<AgentDetailTopBar
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
				onOpenParentChat={() => {}}
				onArchiveAgent={() => {}}
				onUnarchiveAgent={() => {}}
				onArchiveAndDeleteWorkspace={() => {}}
				hasWorkspace={false}
				isSidebarCollapsed={isSidebarCollapsed}
				onToggleSidebarCollapsed={onToggleSidebarCollapsed}
			/>
			<div className="flex flex-1 items-center justify-center text-content-secondary">
				Chat not found
			</div>{" "}
		</div>
	);
};
