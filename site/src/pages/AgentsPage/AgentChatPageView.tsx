import { ArchiveIcon, ArrowDownIcon } from "lucide-react";
import {
	type FC,
	type RefObject,
	useEffect,
	useLayoutEffect,
	useRef,
	useState,
} from "react";
import type { UrlTransform } from "streamdown";
import type * as TypesGen from "#/api/typesGenerated";
import type { ChatDiffStatus, ChatMessagePart } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
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
import { ChatTopBar } from "./components/ChatTopBar";
import { GitPanel } from "./components/GitPanel/GitPanel";
import { RightPanel } from "./components/RightPanel/RightPanel";
import { SidebarTabView } from "./components/Sidebar/SidebarTabView";
import type { ChatDetailError } from "./utils/usageLimitMessage";

type ChatStoreHandle = ReturnType<typeof useChatStore>["store"];

// Re-use the inner presentational components directly. They are

interface EditingState {
	chatInputRef: RefObject<ChatMessageInputRef | null>;
	editorInitialValue: string;
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
	handleContentChange: (content: string) => void;
}

interface AgentChatPageViewProps {
	// Chat data.
	agentId: string;
	chatTitle: string | undefined;
	parentChat: TypesGen.Chat | undefined;
	persistedError: ChatDetailError | undefined;
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
}

export const AgentChatPageView: FC<AgentChatPageViewProps> = ({
	agentId,
	chatTitle,
	parentChat,
	persistedError,
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
					<ScrollAnchoredContainer
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
					</ScrollAnchoredContainer>
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

/**
 * Scroll container that keeps the transcript in normal top-to-bottom
 * document flow while preserving a bottom-anchored chat experience.
 * The user is at the bottom when the remaining scroll distance to the
 * end of the container is within SCROLL_THRESHOLD.
 *
 * Handles:
 * - Loading older message pages via an IntersectionObserver sentinel.
 * - ResizeObserver-driven scroll anchoring for transcript and viewport
 *   size changes.
 * - A floating "Scroll to bottom" button when the user is scrolled
 *   away from the bottom.
 *
 * CSS overflow anchoring is disabled on the container, so all position
 * restoration is done manually.
 */
const SCROLL_THRESHOLD = 100;

function isNearBottom(container: HTMLElement): boolean {
	return (
		container.scrollHeight - container.scrollTop - container.clientHeight <
		SCROLL_THRESHOLD
	);
}

function scrollTranscriptToBottom({
	behavior,
	scrollContainerRef,
	autoScrollRef,
	isRestoringScrollRef,
	restoreGuardRafIdRef,
	setShowScrollToBottom,
}: {
	behavior: "smooth" | "instant";
	scrollContainerRef: RefObject<HTMLDivElement | null>;
	autoScrollRef: { current: boolean };
	isRestoringScrollRef: { current: boolean };
	restoreGuardRafIdRef: { current: number | null };
	setShowScrollToBottom: (next: boolean) => void;
}): void {
	const container = scrollContainerRef.current;
	if (!container) {
		return;
	}

	// Cancel any pending guard-clear so it cannot drop the flag
	// during a smooth scroll animation.
	if (restoreGuardRafIdRef.current !== null) {
		cancelAnimationFrame(restoreGuardRafIdRef.current);
		restoreGuardRafIdRef.current = null;
	}
	autoScrollRef.current = true;
	isRestoringScrollRef.current = true;
	const top = Math.max(container.scrollHeight - container.clientHeight, 0);
	if (behavior === "smooth") {
		container.scrollTo({ top, behavior: "smooth" });
	} else {
		container.scrollTop = top;
		// Instant scrollTop assignment may not fire a scroll event when the
		// container is already at the target position, so clear the restoring
		// guard immediately to avoid blocking subsequent scroll handling.
		isRestoringScrollRef.current = false;
	}
	setShowScrollToBottom(false);
}

const ScrollAnchoredContainer: FC<{
	scrollContainerRef: RefObject<HTMLDivElement | null>;
	scrollToBottomRef: RefObject<(() => void) | null>;
	isFetchingMoreMessages: boolean;
	hasMoreMessages: boolean;
	onFetchMoreMessages: () => void;
	children: React.ReactNode;
}> = ({
	scrollContainerRef,
	scrollToBottomRef,
	isFetchingMoreMessages,
	hasMoreMessages,
	onFetchMoreMessages,
	children,
}) => {
	const sentinelRef = useRef<HTMLDivElement>(null);
	const observerRef = useRef<IntersectionObserver | null>(null);
	const isFetchingRef = useRef(isFetchingMoreMessages);
	const hasFetchedRef = useRef(false);
	const autoScrollRef = useRef(true);
	const contentRef = useRef<HTMLDivElement>(null);
	const pendingPrependRef = useRef<{
		contentHeight: number;
		scrollHeight: number;
		contentWidth: number;
	} | null>(null);
	// Guard flag: true while a programmatic scroll adjustment is in-flight.
	// The scroll handler skips autoScrollRef updates and re-render triggers
	// when this is set, preventing user-visible jitter. Cleared when the
	// scroll reaches its destination or the user actively interrupts.
	const isRestoringScrollRef = useRef(false);
	// Shared guard-clear RAF ID across both the content and container
	// ResizeObservers. Both observers set isRestoringScrollRef before
	// pinning and schedule a one-frame-delayed clear. Without a shared
	// ID, one observer's clear can fire while the other's pin chain is
	// still in-flight, leaving a window where the scroll handler reads
	// stale position data and disables auto-scroll.
	const restoreGuardRafIdRef = useRef<number | null>(null);
	const cancelPendingPinsRef = useRef<(() => void) | null>(null);
	// Guard counter: positive while one or more touch contacts are active.
	// Prevents ResizeObserver callbacks from snapping scroll to bottom
	// during mobile URL bar show/hide, which triggers container resize
	// events while the user's finger is still on the screen.
	const activeTouchCountRef = useRef(0);
	// Guard flag: true while the user is actively scrolling via
	// mouse wheel or trackpad. Set on each wheel event and cleared
	// after a short debounce period. Prevents ResizeObserver
	// callbacks from snapping scroll to bottom during active
	// wheel/trackpad scrolling within the near-bottom threshold.
	const isWheelScrollingRef = useRef(false);
	// Track whether a resize would have pinned to bottom while the
	// wheel guard was active. When scrolling stops, run one catch-up
	// pin so auto-follow resumes without waiting for another resize.
	const pendingWheelPinRef = useRef(false);
	// Snapshot of autoScrollRef at the start of the current wheel
	// burst. Used by the debounce timeout to decide whether to repin
	// after deferred content growth.
	const wheelSessionAutoScrollRef = useRef(false);
	// scrollTop at the start of the current wheel burst. Compared
	// against the final scrollTop to detect intentional upward scrolls.
	const wheelSessionStartTopRef = useRef(0);
	useLayoutEffect(() => {
		isFetchingRef.current = isFetchingMoreMessages;
		if (isFetchingMoreMessages) {
			hasFetchedRef.current = true;
		}
	}, [isFetchingMoreMessages]);
	const [showScrollToBottom, setShowScrollToBottom] = useState(false);

	useEffect(() => {
		scrollToBottomRef.current = () => {
			scrollTranscriptToBottom({
				behavior: "instant",
				scrollContainerRef,
				autoScrollRef,
				isRestoringScrollRef,
				restoreGuardRafIdRef,
				setShowScrollToBottom,
			});
		};
		return () => {
			scrollToBottomRef.current = null;
		};
	}, [scrollContainerRef, scrollToBottomRef]);

	// Sentinel observer, triggers loading older messages.
	// All changing values are read from refs so the observer
	// is created once and never torn down / recreated, which
	// would cause spurious intersection callbacks.
	useEffect(() => {
		const sentinel = sentinelRef.current;
		const container = scrollContainerRef.current;
		if (!sentinel || !container) return;

		const observer = new IntersectionObserver(
			([entry]) => {
				if (entry.isIntersecting && !isFetchingRef.current) {
					const container = scrollContainerRef.current;
					const content = contentRef.current;
					if (container && content) {
						const contentRect = content.getBoundingClientRect();
						// Capture the current viewport snapshot before the fetch so
						// prepended content can be restored after layout updates.
						pendingPrependRef.current = {
							contentHeight: contentRect.height,
							scrollHeight: container.scrollHeight,
							contentWidth: contentRect.width,
						};
					}
					onFetchMoreMessages();
				}
			},
			{
				root: container,
				rootMargin: "600px 0px 0px 0px",
				threshold: 0.01,
			},
		);
		observerRef.current = observer;
		// Defer sentinel observation until after the initial bottom
		// pin settles. In normal flex-col flow, scrollTop starts at 0,
		// which places the sentinel in the viewport and would trigger
		// an eager history fetch before the transcript pins to bottom.
		let deferInnerId: number | null = null;
		const deferOuterId = requestAnimationFrame(() => {
			deferInnerId = requestAnimationFrame(() => {
				observer.observe(sentinel);
			});
		});
		return () => {
			cancelAnimationFrame(deferOuterId);
			if (deferInnerId !== null) {
				cancelAnimationFrame(deferInnerId);
			}
			observer.disconnect();
			observerRef.current = null;
		};
	}, [scrollContainerRef, onFetchMoreMessages]);

	// When a fetch completes, re-observe the sentinel to force
	// the IntersectionObserver to re-evaluate. The observer only
	// fires on state *changes* (entering/leaving), so if the
	// sentinel stayed visible throughout the fetch it won't fire
	// again on its own.
	useEffect(() => {
		if (isFetchingMoreMessages) return;
		// Skip re-observation on initial mount. The sentinel setup
		// effect defers observation via double-RAF to avoid an eager
		// fetch before the initial bottom pin settles. This effect
		// would bypass that defer since isFetchingMoreMessages starts
		// as false.
		if (!hasFetchedRef.current) return;
		const pendingPrepend = pendingPrependRef.current;
		const cleanupId = requestAnimationFrame(() => {
			const container = scrollContainerRef.current;
			if (
				!pendingPrepend ||
				pendingPrependRef.current !== pendingPrepend ||
				!container
			) {
				return;
			}
			// If the fetch did not change the container scroll height, the
			// ResizeObserver never runs to clear the pending prepend snapshot.
			// Clear it after layout settles so later resizes do not apply stale
			// scroll compensation.
			if (Math.abs(container.scrollHeight - pendingPrepend.scrollHeight) < 1) {
				pendingPrependRef.current = null;
			}
		});
		const sentinel = sentinelRef.current;
		const observer = observerRef.current;
		if (sentinel && observer) {
			observer.unobserve(sentinel);
			observer.observe(sentinel);
		}
		return () => {
			cancelAnimationFrame(cleanupId);
		};
	}, [isFetchingMoreMessages, scrollContainerRef]);

	useEffect(() => {
		const container = scrollContainerRef.current;
		const content = contentRef.current;
		if (!container || !content) return;

		const initialContentRect = content.getBoundingClientRect();
		let prevContentHeight = initialContentRect.height;
		let prevContentWidth = initialContentRect.width;
		let pinOuterRafId: number | null = null;
		let pinInnerRafId: number | null = null;

		const cancelPendingPins = () => {
			if (pinOuterRafId !== null) {
				cancelAnimationFrame(pinOuterRafId);
			}
			if (pinInnerRafId !== null) {
				cancelAnimationFrame(pinInnerRafId);
			}
			pinOuterRafId = null;
			pinInnerRafId = null;
		};

		const scheduleBottomPin = () => {
			// If a pin is already in-flight, let it complete. The
			// inner RAF reads scrollHeight at execution time so it
			// always targets the latest bottom. Cancelling and
			// rescheduling on every ResizeObserver notification
			// starves the pin on Safari, where sticky-element
			// repositioning generates extra resize events that
			// perpetually restart the double-RAF chain.
			if (pinOuterRafId !== null || pinInnerRafId !== null) {
				return;
			}
			// Cancel any pending guard-clear from a previous pin
			// chain (or the container resize observer). Without
			// this, the stale guard-clear fires during the new
			// chain's double-RAF window, setting
			// isRestoringScrollRef to false. A scroll event in
			// that gap reads the wrong position and disables
			// auto-scroll, causing the pin to be skipped.
			if (restoreGuardRafIdRef.current !== null) {
				cancelAnimationFrame(restoreGuardRafIdRef.current);
				restoreGuardRafIdRef.current = null;
			}
			pendingWheelPinRef.current = false;
			isRestoringScrollRef.current = true;
			// Double-RAF lets React's commit phase and the browser's
			// layout pass both complete before we pin to bottom.
			pinOuterRafId = requestAnimationFrame(() => {
				pinOuterRafId = null;
				pinInnerRafId = requestAnimationFrame(() => {
					pinInnerRafId = null;
					if (!autoScrollRef.current) {
						isRestoringScrollRef.current = false;
						return;
					}
					if (restoreGuardRafIdRef.current !== null) {
						cancelAnimationFrame(restoreGuardRafIdRef.current);
						restoreGuardRafIdRef.current = null;
					}
					container.scrollTop = Math.max(
						container.scrollHeight - container.clientHeight,
						0,
					);
					setShowScrollToBottom(false);
					restoreGuardRafIdRef.current = requestAnimationFrame(() => {
						restoreGuardRafIdRef.current = null;
						// Content may have grown between the pin and
						// this callback. Re-pin instead of dropping the
						// guard if we drifted off-bottom.
						if (!isNearBottom(container)) {
							scheduleBottomPin();
							return;
						}
						isRestoringScrollRef.current = false;
					});
				});
			});
		};

		cancelPendingPinsRef.current = cancelPendingPins;
		const observer = new ResizeObserver((entries) => {
			const entry = entries[0];
			const nextHeight =
				entry?.contentRect.height ?? content.getBoundingClientRect().height;
			const nextWidth =
				entry?.contentRect.width ?? content.getBoundingClientRect().width;
			const delta = nextHeight - prevContentHeight;
			const widthChanged = Math.abs(nextWidth - prevContentWidth) > 1;
			prevContentHeight = nextHeight;
			prevContentWidth = nextWidth;

			const pending = pendingPrependRef.current;
			if (pending !== null && isFetchingRef.current) {
				pending.contentHeight = nextHeight;
				pending.scrollHeight = container.scrollHeight;
				pending.contentWidth = nextWidth;
				return;
			}
			if (Math.abs(delta) < 1) {
				return;
			}

			// Restore the viewport after older messages are prepended.
			if (pending !== null && !isFetchingRef.current) {
				pendingPrependRef.current = null;
				// Width changes indicate reflow rather than a true prepend.
				if (!widthChanged) {
					const scrollHeightDelta =
						container.scrollHeight - pending.scrollHeight;
					if (scrollHeightDelta > 0) {
						if (restoreGuardRafIdRef.current !== null) {
							cancelAnimationFrame(restoreGuardRafIdRef.current);
						}
						isRestoringScrollRef.current = true;
						container.scrollTop = container.scrollTop + scrollHeightDelta;
						restoreGuardRafIdRef.current = requestAnimationFrame(() => {
							isRestoringScrollRef.current = false;
							restoreGuardRafIdRef.current = null;
						});
					}
				}
				return;
			}

			if (autoScrollRef.current && activeTouchCountRef.current === 0) {
				if (isWheelScrollingRef.current) {
					pendingWheelPinRef.current = true;
					return;
				}
				scheduleBottomPin();
				return;
			}

			// Skip compensation during reflow. Width changes indicate the
			// height delta is distributed through the transcript rather than
			// appended at the bottom, so applying the full delta would
			// overcompensate and jump the user.
			if (widthChanged) {
				return;
			}

			// In normal flow, appends grow below the viewport, so users reading
			// history do not need scroll compensation.
		});
		observer.observe(content);

		// In normal flex-col flow, scrollTop starts at 0 (top).
		// Pin to bottom on initial mount so existing chats open
		// at the most recent messages.
		if (autoScrollRef.current) {
			scheduleBottomPin();
		}

		return () => {
			cancelPendingPinsRef.current = null;
			observer.disconnect();
			cancelPendingPins();
			if (restoreGuardRafIdRef.current !== null) {
				cancelAnimationFrame(restoreGuardRafIdRef.current);
				restoreGuardRafIdRef.current = null;
			}
			isRestoringScrollRef.current = false;
		};
	}, [scrollContainerRef]);

	useEffect(() => {
		const container = scrollContainerRef.current;
		if (!container) return;

		let prevContainerHeight = container.clientHeight;

		const observer = new ResizeObserver((entries) => {
			const nextHeight =
				entries[0]?.contentRect.height ?? container.clientHeight;
			const delta = nextHeight - prevContainerHeight;
			prevContainerHeight = nextHeight;
			if (Math.abs(delta) < 1 || !autoScrollRef.current) {
				return;
			}
			if (activeTouchCountRef.current > 0) {
				return;
			}
			if (isWheelScrollingRef.current) {
				pendingWheelPinRef.current = true;
				return;
			}

			if (restoreGuardRafIdRef.current !== null) {
				cancelAnimationFrame(restoreGuardRafIdRef.current);
			}
			isRestoringScrollRef.current = true;
			container.scrollTop = Math.max(
				container.scrollHeight - container.clientHeight,
				0,
			);
			restoreGuardRafIdRef.current = requestAnimationFrame(() => {
				restoreGuardRafIdRef.current = null;
				if (!isNearBottom(container)) {
					// Content grew between the pin and this frame.
					// Re-pin directly since scheduleBottomPin is in
					// a different effect closure.
					container.scrollTop = Math.max(
						container.scrollHeight - container.clientHeight,
						0,
					);
					restoreGuardRafIdRef.current = requestAnimationFrame(() => {
						restoreGuardRafIdRef.current = null;
						isRestoringScrollRef.current = false;
					});
					return;
				}
				isRestoringScrollRef.current = false;
			});
		});
		observer.observe(container);
		return () => {
			observer.disconnect();
			// The shared restoreGuardRafIdRef is cancelled by the
			// content observer's cleanup (same dependency array, React
			// runs cleanups in declaration order). Reset the guard
			// defensively so this effect is self-contained if the
			// ordering ever changes.
			isRestoringScrollRef.current = false;
		};
	}, [scrollContainerRef]);

	// Track scroll position to show/hide the scroll-to-bottom button.
	useEffect(() => {
		const container = scrollContainerRef.current;
		if (!container) return;

		let rafId: number | null = null;
		let wheelTimeoutId: ReturnType<typeof setTimeout> | null = null;

		const handleScroll = () => {
			// While a programmatic scroll is in progress (e.g. smooth
			// scroll-to-bottom), suppress normal handling. Clear the
			// guard once the scroll reaches the bottom so normal
			// tracking resumes. User-input interruptions are handled
			// separately via wheel/touchstart/pointerdown listeners.
			if (isRestoringScrollRef.current) {
				if (isNearBottom(container)) {
					isRestoringScrollRef.current = false;
					autoScrollRef.current = true;
					setShowScrollToBottom(false);
				}
				return;
			}

			const nearBottom = isNearBottom(container);
			// Only ENABLE follow mode from the scroll handler.
			// Never disable it here. WebKit internally adjusts
			// scrollTop during layout when content above the
			// viewport changes height (a form of scroll anchoring
			// it applies even with overflow-anchor:none). These
			// browser-initiated adjustments fire scroll events
			// that see isNearBottom=false and would permanently
			// kill follow mode. Disabling follow is exclusive to
			// user-interaction handlers (wheel, touch, scrollbar
			// pointer) which call handleUserInterrupt directly.
			if (nearBottom) {
				autoScrollRef.current = true;
			} else if (
				autoScrollRef.current &&
				!isWheelScrollingRef.current &&
				activeTouchCountRef.current === 0
			) {
				// Follow mode is on but we are not at bottom, and
				// the user is not actively scrolling. This indicates
				// WebKit adjusted scrollTop during layout (a form of
				// scroll anchoring it applies even with
				// overflow-anchor:none). Re-pin immediately and keep
				// the restore guard up so the next scroll event from
				// this pin is suppressed.
				isRestoringScrollRef.current = true;
				container.scrollTop = Math.max(
					container.scrollHeight - container.clientHeight,
					0,
				);
				if (restoreGuardRafIdRef.current !== null) {
					cancelAnimationFrame(restoreGuardRafIdRef.current);
				}
				restoreGuardRafIdRef.current = requestAnimationFrame(() => {
					restoreGuardRafIdRef.current = null;
					isRestoringScrollRef.current = false;
				});
				return;
			}

			// Throttle the button visibility state update to once per
			// frame. This is the only part that triggers a re-render.
			if (rafId !== null) return;
			rafId = requestAnimationFrame(() => {
				setShowScrollToBottom((prev) => {
					const shouldShow = !isNearBottom(container);
					return prev === shouldShow ? prev : shouldShow;
				});
				rafId = null;
			});
		};

		const handleUserInterrupt = () => {
			// Always clear the restoration guard so the next scroll event is
			// processed normally.
			isRestoringScrollRef.current = false;
			// Only disable auto-scroll when the user is away from the bottom.
			// Trackpad noise or accidental wheel events at the bottom should
			// not break streaming follow-mode.
			if (!isNearBottom(container)) {
				autoScrollRef.current = false;
				pendingWheelPinRef.current = false;
				cancelPendingPinsRef.current?.();
			}
		};

		const getChangedTouchCount = (event: TouchEvent) => {
			// A single touch event can add or remove multiple contacts.
			// Count changed touches so the guard stays active until the
			// final finger leaves the screen.
			return Math.max(event.changedTouches.length, 1);
		};

		const handleWheel = () => {
			if (!isWheelScrollingRef.current) {
				// First wheel event of this burst: snapshot the current
				// follow state and scroll position so the debounce
				// timeout can distinguish content-driven gaps from
				// intentional user scroll.
				wheelSessionAutoScrollRef.current = autoScrollRef.current;
				wheelSessionStartTopRef.current = container.scrollTop;
			}
			isWheelScrollingRef.current = true;
			if (wheelTimeoutId !== null) {
				clearTimeout(wheelTimeoutId);
			}
			// Clear the wheel-scrolling flag after 150ms of inactivity.
			// This covers trackpad momentum and rapid discrete wheel
			// ticks without permanently blocking ResizeObserver pins.
			wheelTimeoutId = setTimeout(() => {
				isWheelScrollingRef.current = false;
				wheelTimeoutId = null;
				const wasPinDeferred = pendingWheelPinRef.current;
				pendingWheelPinRef.current = false;
				// Repin if the wheel burst started in follow mode and
				// content grew while the guard was active, unless the
				// user clearly scrolled away from the near-bottom follow
				// zone during the burst.
				if (wasPinDeferred && wheelSessionAutoScrollRef.current) {
					const scrolledUp =
						container.scrollTop < wheelSessionStartTopRef.current - 1;
					const nearBottom = isNearBottom(container);
					if (!scrolledUp || nearBottom) {
						scrollTranscriptToBottom({
							behavior: "instant",
							scrollContainerRef,
							autoScrollRef,
							isRestoringScrollRef,
							restoreGuardRafIdRef,
							setShowScrollToBottom,
						});
					} else {
						autoScrollRef.current = false;
						setShowScrollToBottom(true);
					}
				} else {
					// Sync follow state with the actual scroll position
					// now that the wheel burst is over.
					const nearBottom = isNearBottom(container);
					autoScrollRef.current = nearBottom;
					setShowScrollToBottom(!nearBottom);
				}
			}, 150);
			// Clear the restoration guard so user input can interrupt
			// programmatic scrolls, but do not call handleUserInterrupt()
			// here. The scroll handler and debounce timeout manage
			// follow-mode transitions to avoid races with deferred
			// content growth.
			isRestoringScrollRef.current = false;
			cancelPendingPinsRef.current?.();
		};

		const handleTouchStart = (event: TouchEvent) => {
			activeTouchCountRef.current += getChangedTouchCount(event);
			handleUserInterrupt();
		};

		const handleTouchEnd = (event: TouchEvent) => {
			activeTouchCountRef.current = Math.max(
				0,
				activeTouchCountRef.current - getChangedTouchCount(event),
			);
			if (activeTouchCountRef.current === 0) {
				// Re-evaluate because momentum scrolling may carry the
				// user away from the bottom after touchstart initially
				// saw them as near the bottom.
				if (!isNearBottom(container)) {
					autoScrollRef.current = false;
					cancelPendingPinsRef.current?.();
				}
			}
		};

		container.addEventListener("scroll", handleScroll, { passive: true });
		container.addEventListener("wheel", handleWheel, {
			passive: true,
		});
		// Scrollbar-track drags don't fire wheel or touch events.
		// Detect them via pointerdown on the container element
		// itself (content clicks target child elements, scrollbar
		// clicks target the scroller). This is the only non-wheel,
		// non-touch user interaction that scrolls.
		const handlePointerDown = (event: PointerEvent) => {
			if (event.target === container) {
				handleUserInterrupt();
			}
		};
		container.addEventListener("pointerdown", handlePointerDown, {
			passive: true,
		});
		container.addEventListener("touchstart", handleTouchStart, {
			passive: true,
		});
		container.addEventListener("touchend", handleTouchEnd, {
			passive: true,
		});
		// touchcancel fires when the OS interrupts a gesture (e.g.,
		// incoming call, system gesture). Must also decrement the
		// touch counter to avoid a stuck positive value.
		container.addEventListener("touchcancel", handleTouchEnd, {
			passive: true,
		});
		// Reset touch counter when page is hidden (e.g., tab switch).
		// The browser may not fire touchend/touchcancel when the user
		// switches away mid-gesture, which would leave the counter
		// positive and permanently block ResizeObserver pins.
		const handleVisibilityChange = () => {
			if (document.hidden) {
				activeTouchCountRef.current = 0;
			}
		};
		document.addEventListener("visibilitychange", handleVisibilityChange);
		return () => {
			container.removeEventListener("scroll", handleScroll);
			container.removeEventListener("wheel", handleWheel);
			container.removeEventListener("pointerdown", handlePointerDown);
			container.removeEventListener("touchstart", handleTouchStart);
			container.removeEventListener("touchend", handleTouchEnd);
			container.removeEventListener("touchcancel", handleTouchEnd);
			document.removeEventListener("visibilitychange", handleVisibilityChange);
			if (rafId !== null) {
				cancelAnimationFrame(rafId);
			}
			if (wheelTimeoutId !== null) {
				clearTimeout(wheelTimeoutId);
			}
		};
	}, [scrollContainerRef]);

	const handleScrollToBottom = () => {
		scrollTranscriptToBottom({
			behavior: "smooth",
			scrollContainerRef,
			autoScrollRef,
			isRestoringScrollRef,
			restoreGuardRafIdRef,
			setShowScrollToBottom,
		});
	};

	return (
		<div className="relative flex min-h-0 flex-1 flex-col">
			<div
				ref={scrollContainerRef}
				data-testid="scroll-container"
				className="flex min-h-0 flex-1 flex-col overflow-y-auto [overflow-anchor:none] [overscroll-behavior:contain] [scrollbar-gutter:stable] [scrollbar-width:thin] [scrollbar-color:hsl(var(--surface-quaternary))_transparent]"
			>
				<div ref={contentRef}>
					{hasMoreMessages && (
						<div ref={sentinelRef} className="h-px shrink-0" />
					)}
					{children}
				</div>
			</div>
			<div className="pointer-events-none absolute inset-x-0 bottom-2 z-10 flex justify-center overflow-y-auto py-2 [scrollbar-gutter:stable] [scrollbar-width:thin]">
				<Button
					variant="outline"
					size="icon"
					className={cn(
						"rounded-full bg-surface-primary shadow-md transition-all duration-200",
						showScrollToBottom
							? "pointer-events-auto translate-y-0 opacity-100"
							: "translate-y-2 opacity-0",
					)}
					onClick={handleScrollToBottom}
					aria-label="Scroll to bottom"
					aria-hidden={!showScrollToBottom || undefined}
					tabIndex={showScrollToBottom ? undefined : -1}
				>
					<ArrowDownIcon />
				</Button>
			</div>
		</div>
	);
};
