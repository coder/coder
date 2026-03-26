import { ArchiveIcon, ArrowDownIcon } from "lucide-react";
import { type FC, type RefObject, useEffect, useRef, useState } from "react";
import type { UrlTransform } from "streamdown";
import { cn } from "utils/cn";
import { pageTitle } from "utils/page";
import type * as TypesGen from "#/api/typesGenerated";
import type { ChatDiffStatus, ChatMessagePart } from "#/api/typesGenerated";
import type { ModelSelectorOption } from "#/components/ai-elements";
import { Button } from "#/components/Button/Button";
import type { ChatDetailError } from "../utils/usageLimitMessage";
import { AgentChatInput, type ChatMessageInputRef } from "./AgentChatInput";
import type { useChatStore } from "./AgentDetail/ChatContext";
import { AgentDetailTopBar } from "./AgentDetail/TopBar";
import { AgentDetailInput, AgentDetailTimeline } from "./AgentDetailContent";
import {
	ChatConversationSkeleton,
	RightPanelSkeleton,
} from "./AgentsSkeletons";
import { GitPanel } from "./GitPanel/GitPanel";
import { RightPanel } from "./RightPanel/RightPanel";
import { SidebarTabView } from "./Sidebar/SidebarTabView";

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

interface AgentDetailViewProps {
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

export const AgentDetailView: FC<AgentDetailViewProps> = ({
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
	scrollContainerRef,
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

	// Compute local diff stats from git watcher unified diffs.

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
					"relative flex min-h-0 min-w-0 flex-1 flex-col overflow-x-hidden",
					visualExpanded && "hidden",
					shouldShowSidebar && "max-md:hidden",
				)}
			>
				<div className="relative z-10 shrink-0 overflow-visible">
					<AgentDetailTopBar
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
						onArchiveAndDeleteWorkspace={handleArchiveAndDeleteWorkspaceAction}
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
					isFetchingMoreMessages={isFetchingMoreMessages}
					hasMoreMessages={hasMoreMessages}
					onFetchMoreMessages={onFetchMoreMessages}
				>
					<div className="px-4">
						<AgentDetailTimeline
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
						isModelCatalogLoading={isModelCatalogLoading}
						inputRef={editing.chatInputRef}
						initialValue={editing.editorInitialValue}
						onContentChange={editing.handleContentChange}
						editingQueuedMessageID={editing.editingQueuedMessageID}
						onStartQueueEdit={editing.handleStartQueueEdit}
						onCancelQueueEdit={editing.handleCancelQueueEdit}
						isEditingHistoryMessage={editing.editingMessageId !== null}
						onCancelHistoryEdit={editing.handleCancelHistoryEdit}
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
	isModelCatalogLoading?: boolean;
	isSidebarCollapsed: boolean;
	onToggleSidebarCollapsed: () => void;
	showRightPanel: boolean;
}

export const AgentDetailLoadingView: FC<AgentDetailLoadingViewProps> = ({
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
					onArchiveAgent={() => {}}
					onUnarchiveAgent={() => {}}
					onArchiveAndDeleteWorkspace={() => {}}
					hasWorkspace={false}
					isSidebarCollapsed={isSidebarCollapsed}
					onToggleSidebarCollapsed={onToggleSidebarCollapsed}
				/>
				<div className="flex min-h-0 flex-1 flex-col-reverse overflow-y-auto [scrollbar-gutter:stable] [scrollbar-width:thin] [scrollbar-color:hsl(var(--surface-quaternary))_transparent]">
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
				onArchiveAgent={() => {}}
				onUnarchiveAgent={() => {}}
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
 * Scroll container that uses flex-col-reverse for bottom-anchored chat
 * layout. In this layout scrollTop = 0 means the user is at the
 * bottom (most recent content); scrolling up moves scrollTop away from
 * 0 (negative in Chrome, positive in Firefox).
 *
 * Handles:
 * - Loading older message pages via an IntersectionObserver sentinel.
 * - ResizeObserver-driven scroll anchoring for transcript and viewport
 *   size changes.
 * - A floating "Scroll to bottom" button when the user is scrolled
 *   away from the bottom.
 *
 * CSS scroll anchoring is unreliable in flex-col-reverse containers,
 * so all position restoration is done manually.
 */
const SCROLL_THRESHOLD = 100;

// In flex-col-reverse, scrollTop is 0 at the bottom. Its sign
// when scrolled up varies by engine (negative in Chrome, positive
// in Firefox). The user is "near bottom" when close to 0.
function isNearBottom(container: HTMLElement): boolean {
	return Math.abs(container.scrollTop) < SCROLL_THRESHOLD;
}

const ScrollAnchoredContainer: FC<{
	scrollContainerRef: RefObject<HTMLDivElement | null>;
	isFetchingMoreMessages: boolean;
	hasMoreMessages: boolean;
	onFetchMoreMessages: () => void;
	children: React.ReactNode;
}> = ({
	scrollContainerRef,
	isFetchingMoreMessages,
	hasMoreMessages,
	onFetchMoreMessages,
	children,
}) => {
	const sentinelRef = useRef<HTMLDivElement>(null);
	const observerRef = useRef<IntersectionObserver | null>(null);
	const isFetchingRef = useRef(isFetchingMoreMessages);
	const onFetchRef = useRef(onFetchMoreMessages);
	const autoScrollRef = useRef(true);
	const contentRef = useRef<HTMLDivElement>(null);
	// Guard flag: true while a programmatic scroll adjustment is in-flight.
	// The scroll handler skips autoScrollRef updates and re-render triggers
	// when this is set, preventing user-visible jitter. Cleared when the
	// scroll reaches its destination or the user actively interrupts.
	const isRestoringScrollRef = useRef(false);
	useEffect(() => {
		isFetchingRef.current = isFetchingMoreMessages;
		onFetchRef.current = onFetchMoreMessages;
	}, [isFetchingMoreMessages, onFetchMoreMessages]);
	const [showScrollToBottom, setShowScrollToBottom] = useState(false);

	// Sentinel observer — triggers loading older messages.
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
					onFetchRef.current();
				}
			},
			{
				root: container,
				rootMargin: "600px 0px 0px 0px",
				threshold: 0.01,
			},
		);
		observerRef.current = observer;
		observer.observe(sentinel);
		return () => {
			observer.disconnect();
			observerRef.current = null;
		};
	}, [scrollContainerRef]);

	// When a fetch completes, re-observe the sentinel to force
	// the IntersectionObserver to re-evaluate. The observer only
	// fires on state *changes* (entering/leaving), so if the
	// sentinel stayed visible throughout the fetch it won't fire
	// again on its own.
	useEffect(() => {
		if (isFetchingMoreMessages) return;
		const sentinel = sentinelRef.current;
		const observer = observerRef.current;
		if (!sentinel || !observer) return;
		observer.unobserve(sentinel);
		observer.observe(sentinel);
	}, [isFetchingMoreMessages]);

	useEffect(() => {
		const container = scrollContainerRef.current;
		const content = contentRef.current;
		if (!container || !content) return;

		const initialContentRect = content.getBoundingClientRect();
		let prevContentHeight = initialContentRect.height;
		let prevContentWidth = initialContentRect.width;
		let pinOuterRafId: number | null = null;
		let pinInnerRafId: number | null = null;
		let restoreGuardRafId: number | null = null;

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
			cancelPendingPins();
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
					if (restoreGuardRafId !== null) {
						cancelAnimationFrame(restoreGuardRafId);
					}
					container.scrollTop = 0;
					restoreGuardRafId = requestAnimationFrame(() => {
						isRestoringScrollRef.current = false;
						restoreGuardRafId = null;
					});
				});
			});
		};

		const compensateScroll = (delta: number) => {
			if (restoreGuardRafId !== null) {
				cancelAnimationFrame(restoreGuardRafId);
			}
			isRestoringScrollRef.current = true;
			// In flex-col-reverse, "away from bottom" can be either
			// negative (Chrome) or positive (Firefox). Detect which
			// convention applies and compensate accordingly.
			if (container.scrollTop < 0) {
				// Negative convention: subtract to move away from 0 (bottom).
				container.scrollTop -= delta;
			} else {
				// Positive convention: add to move away from 0 (bottom).
				container.scrollTop += delta;
			}
			restoreGuardRafId = requestAnimationFrame(() => {
				isRestoringScrollRef.current = false;
				restoreGuardRafId = null;
			});
		};

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
			if (Math.abs(delta) < 1) {
				return;
			}

			// Skip compensation during pagination. Older messages are
			// prepended in flex-col-reverse which grows content into the
			// overflow direction; the browser preserves scrollTop for us.
			if (isFetchingRef.current) {
				return;
			}

			// Skip compensation during reflow. Width changes indicate the
			// height delta is distributed through the transcript rather than
			// appended at the bottom, so applying the full delta would
			// overcompensate and jump the user.
			if (widthChanged) {
				return;
			}

			if (autoScrollRef.current) {
				scheduleBottomPin();
				return;
			}

			compensateScroll(delta);
		});
		observer.observe(content);

		return () => {
			observer.disconnect();
			cancelPendingPins();
			if (restoreGuardRafId !== null) {
				cancelAnimationFrame(restoreGuardRafId);
			}
			isRestoringScrollRef.current = false;
		};
	}, [scrollContainerRef]);

	useEffect(() => {
		const container = scrollContainerRef.current;
		if (!container) return;

		let prevContainerHeight = container.clientHeight;
		let restoreGuardRafId: number | null = null;

		const observer = new ResizeObserver((entries) => {
			const nextHeight =
				entries[0]?.contentRect.height ?? container.clientHeight;
			const delta = nextHeight - prevContainerHeight;
			prevContainerHeight = nextHeight;
			if (Math.abs(delta) < 1 || !autoScrollRef.current) {
				return;
			}

			if (restoreGuardRafId !== null) {
				cancelAnimationFrame(restoreGuardRafId);
			}
			isRestoringScrollRef.current = true;
			container.scrollTop = 0;
			restoreGuardRafId = requestAnimationFrame(() => {
				isRestoringScrollRef.current = false;
				restoreGuardRafId = null;
			});
		});
		observer.observe(container);

		return () => {
			observer.disconnect();
			if (restoreGuardRafId !== null) {
				cancelAnimationFrame(restoreGuardRafId);
			}
			isRestoringScrollRef.current = false;
		};
	}, [scrollContainerRef]);

	// Track scroll position to show/hide the scroll-to-bottom button.
	// In a flex-col-reverse container, scrollTop = 0 means the user
	// is at the bottom (most recent content). Scrolling up moves
	// scrollTop away from 0, with the sign varying by engine.
	useEffect(() => {
		const container = scrollContainerRef.current;
		if (!container) return;

		let rafId: number | null = null;

		const handleScroll = () => {
			// While a programmatic scroll is in progress (e.g. smooth
			// scroll-to-bottom), suppress normal handling. Clear the
			// guard once the scroll reaches the bottom so normal
			// tracking resumes. User-input interruptions are handled
			// separately via wheel/touchstart listeners.
			if (isRestoringScrollRef.current) {
				if (isNearBottom(container)) {
					isRestoringScrollRef.current = false;
					autoScrollRef.current = true;
				}
				return;
			}

			const nearBottom = isNearBottom(container);
			autoScrollRef.current = nearBottom;

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
			if (isRestoringScrollRef.current) {
				isRestoringScrollRef.current = false;
			}
		};

		container.addEventListener("scroll", handleScroll, { passive: true });
		container.addEventListener("wheel", handleUserInterrupt, {
			passive: true,
		});
		container.addEventListener("touchstart", handleUserInterrupt, {
			passive: true,
		});
		return () => {
			container.removeEventListener("scroll", handleScroll);
			container.removeEventListener("wheel", handleUserInterrupt);
			container.removeEventListener("touchstart", handleUserInterrupt);
			if (rafId !== null) {
				cancelAnimationFrame(rafId);
			}
		};
	}, [scrollContainerRef]);

	const handleScrollToBottom = () => {
		const container = scrollContainerRef.current;
		if (!container) return;
		autoScrollRef.current = true;
		isRestoringScrollRef.current = true;
		container.scrollTo({ top: 0, behavior: "smooth" });
		// Hide immediately so the button doesn't linger while the
		// smooth scroll animates. If the user interrupts the scroll
		// before it reaches the bottom, the scroll handler will
		// re-show the button.
		setShowScrollToBottom(false);
	};

	return (
		<div className="relative flex min-h-0 flex-1 flex-col">
			<div
				ref={scrollContainerRef}
				data-testid="scroll-container"
				className="flex min-h-0 flex-1 flex-col-reverse overflow-y-auto [overflow-anchor:none] [scrollbar-gutter:stable] [scrollbar-width:thin] [scrollbar-color:hsl(var(--surface-quaternary))_transparent]"
			>
				<div ref={contentRef}>{children}</div>
				{hasMoreMessages && <div ref={sentinelRef} className="h-px shrink-0" />}
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
