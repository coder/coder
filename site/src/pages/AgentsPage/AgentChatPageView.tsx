import { ArchiveIcon, TriangleAlertIcon } from "lucide-react";

import {
	type FC,
	type ReactNode,
	type RefObject,
	useEffect,
	useRef,
	useState,
} from "react";
import { useQueryClient } from "react-query";
import type { UrlTransform } from "streamdown";
import { v4 as uuidv4 } from "uuid";
import { chatDiffContentsKey } from "#/api/queries/chats";
import type * as TypesGen from "#/api/typesGenerated";
import type {
	AgentChatSendShortcut,
	ChatDiffStatus,
	ChatMessagePart,
} from "#/api/typesGenerated";
import { useProxy } from "#/contexts/ProxyContext";
import { isWorkspaceAppEmbeddable } from "#/modules/apps/apps";
import { WorkspaceAppFrame } from "#/modules/apps/WorkspaceAppFrame";
import { findWorkspaceAppWithAgent } from "#/modules/apps/workspaceApps";
import { cn } from "#/utils/cn";
import { pageTitle } from "#/utils/page";
import { findWorkspaceAgent } from "#/utils/workspace";
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
import { ChatSharingPopoverContent } from "./components/ChatSharingPopover";
import { getEffectiveTabId } from "./components/ChatsSidebar/tabs/getEffectiveTabId";
import { SidebarTabView } from "./components/ChatsSidebar/tabs/SidebarTabView";
import { ChatTopBar } from "./components/ChatTopBar";
import { GitPanel } from "./components/GitPanel/GitPanel";
import { DebugPanel } from "./components/RightPanel/DebugPanel/DebugPanel";
import { DesktopPanel } from "./components/RightPanel/DesktopPanel";
import { PortPreviewPanel } from "./components/RightPanel/PortPreviewPanel";
import { RightPanel } from "./components/RightPanel/RightPanel";
import { RightPanelAddTabControl } from "./components/RightPanel/RightPanelAddTabControl";
import { getWorkspaceStatus, StatusIcon } from "./components/StatusIcon";
import { TerminalPanel } from "./components/TerminalPanel";
import { ChatWorkspaceContext } from "./context/ChatWorkspaceContext";
import { chatWidthClass, useChatFullWidth } from "./hooks/useChatFullWidth";
import {
	getPersistedDefaultTerminalHidden,
	getPersistedRightPanelTabs,
	savePersistedDefaultTerminalHidden,
	savePersistedRightPanelTabs,
} from "./utils/rightPanelTabStorage";
import {
	type PortSelection,
	type UserRightPanelTab,
	validateUserRightPanelTabs,
} from "./utils/rightPanelTabs";
import {
	getPersistedSidebarTabId,
	savePersistedSidebarTabId,
} from "./utils/sidebarTabStorage";
import type { ChatDetailError } from "./utils/usageLimitMessage";

type ChatStoreHandle = ReturnType<typeof useChatStore>["store"];

type ChatOwnerInfo = {
	name?: string;
	username?: string;
};

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
	sendShortcut: AgentChatSendShortcut;
	organizationId: string | undefined;
	chatTitle: string | undefined;
	parentChat: TypesGen.Chat | undefined;
	persistedError: ChatDetailError | undefined;
	isArchived: boolean;
	isSharedChat: boolean;
	chatOwner: ChatOwnerInfo | undefined;
	canShareChat: boolean;
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
	canConfigureAgentSetup: boolean;
	providerCount?: number;
	modelCount?: number;
	unsupportedProviderNames?: readonly string[];
	aiGatewayDisabled?: boolean;
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
		hasReceivedChanges: boolean;

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

	// Chat actions.
	handleArchiveAgentAction: () => void;
	handleUnarchiveAgentAction: () => void;
	handleArchiveAndDeleteWorkspaceAction: () => void;
	handlePinAgentAction?: () => void;
	handleUnpinAgentAction?: () => void;
	handleOpenRenameDialogAction?: () => void;
	isPinned?: boolean;
	isChildChat?: boolean;
	isArchivingThisChat?: boolean;

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

	chatContext?: TypesGen.ChatContext;
}

const UnavailableTabMessage: FC<{ message: string }> = ({ message }) => (
	<div className="flex h-full min-h-0 items-center justify-center px-6 text-center text-xs text-content-secondary">
		{message}
	</div>
);

interface UserTabContentProps {
	tab: UserRightPanelTab;
	chatId: string;
	workspace: TypesGen.Workspace | undefined;
	workspaceAgent: TypesGen.WorkspaceAgent | undefined;
	wildcardHostname: string;
	sidebarVisible: boolean;
	isActive: boolean;
	isPending: boolean;
	onTerminalReady: (tabId: string) => void;
}

const UserTabContent: FC<UserTabContentProps> = ({
	tab,
	chatId,
	workspace,
	workspaceAgent,
	wildcardHostname,
	sidebarVisible,
	isActive,
	isPending,
	onTerminalReady,
}) => {
	switch (tab.kind) {
		case "terminal":
			return workspace && workspaceAgent ? (
				<TerminalPanel
					chatId={chatId}
					reconnectionToken={tab.reconnectionToken}
					initialCommand={tab.initialCommand}
					isHot={sidebarVisible && (isActive || isPending)}
					autoFocus={sidebarVisible && isActive}
					onReady={() => onTerminalReady(tab.id)}
					workspace={workspace}
					workspaceAgent={workspaceAgent}
				/>
			) : (
				<UnavailableTabMessage message="Terminal will be available once the workspace agent is ready." />
			);
		case "workspace_app": {
			if (!workspace) {
				return null;
			}
			const app = findWorkspaceAppWithAgent(workspace, tab.agentId, tab.appId);
			if (!app || !isWorkspaceAppEmbeddable(app)) {
				return (
					<UnavailableTabMessage message="This workspace app is no longer available as a right-panel tab." />
				);
			}
			return (
				<WorkspaceAppFrame workspace={workspace} app={app} active={isActive} />
			);
		}
		case "port": {
			if (!workspace) {
				return null;
			}
			const agent = findWorkspaceAgent(workspace, tab.agentId);
			if (!agent) {
				return (
					<UnavailableTabMessage message="This port preview tab is no longer available." />
				);
			}
			return (
				<PortPreviewPanel
					workspace={workspace}
					agent={agent}
					host={wildcardHostname}
					tab={tab}
				/>
			);
		}
		default: {
			const _exhaustive: never = tab;
			return _exhaustive;
		}
	}
};

export const AgentChatPageView: FC<AgentChatPageViewProps> = ({
	agentId,
	sendShortcut,
	organizationId,
	chatTitle,
	parentChat,
	persistedError,
	isArchived,
	isSharedChat,
	chatOwner,
	canShareChat,
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
	canConfigureAgentSetup,
	providerCount,
	modelCount,
	unsupportedProviderNames,
	aiGatewayDisabled,
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
	onWorkspaceChange,
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
	handlePinAgentAction,
	handleUnpinAgentAction,
	handleOpenRenameDialogAction,
	isPinned,
	isChildChat,
	isArchivingThisChat,
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
	chatContext,
}) => {
	const queryClient = useQueryClient();
	const { proxy } = useProxy();
	const wildcardHostname = proxy.preferredWildcardHostname;

	const canOpenChatSharing = canShareChat && organizationId !== undefined;

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
	const [userRightPanelTabs, setUserRightPanelTabsState] = useState<
		UserRightPanelTab[]
	>(() => getPersistedRightPanelTabs(agentId));
	const [defaultTerminalHidden, setDefaultTerminalHiddenState] =
		useState<boolean>(() => getPersistedDefaultTerminalHidden(agentId));
	const [pendingTabId, setPendingTabId] = useState<string | null>(null);

	const setSidebarTabId = (tabId: string) => {
		setSidebarTabIdState(tabId);
		if (!isArchived) {
			savePersistedSidebarTabId(agentId, tabId);
		}
	};

	useEffect(() => {
		if (!isArchived) {
			savePersistedRightPanelTabs(agentId, userRightPanelTabs);
		}
	}, [agentId, isArchived, userRightPanelTabs]);

	useEffect(() => {
		if (!isArchived) {
			savePersistedDefaultTerminalHidden(agentId, defaultTerminalHidden);
		}
	}, [agentId, defaultTerminalHidden, isArchived]);

	const handleOpenDesktop = () => {
		onSetShowSidebarPanel(true);
		setPendingTabId(null);
		setSidebarTabId("desktop");
	};

	const desktopPanelCtx = {
		desktopChatId,
		onOpenDesktop: desktopChatId ? handleOpenDesktop : undefined,
	};

	const shouldShowSidebar = showSidebarPanel;

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

	// Desktop is only available when the workspace and agent are ready;
	// include it in the tab list on that same condition to avoid selecting
	// "desktop" when no desktop panel is rendered.
	const availableDesktopChatId =
		workspace && workspaceAgent ? desktopChatId : undefined;

	const validatedUserRightPanelTabs = validateUserRightPanelTabs(
		userRightPanelTabs,
		{ workspace, workspaceAgent, wildcardHostname },
	);

	const hasBuiltInTerminal = Boolean(
		workspace && workspaceAgent && !defaultTerminalHidden,
	);
	// Single source of truth for available tabs and their order. The list
	// of tab IDs used by `getEffectiveTabId` is derived from this so a
	// new tab can never be added to one without the other going out of
	// sync. Desktop is ordered before terminals so terminals are rightmost.
	const builtInSidebarTabConfigs = [
		{ id: "git", label: "Git" },
		...(debugLoggingEnabled ? [{ id: "debug", label: "Debug" }] : []),
		...(availableDesktopChatId ? [{ id: "desktop", label: "Desktop" }] : []),
		...(hasBuiltInTerminal ? [{ id: "terminal", label: "Terminal" }] : []),
	];
	// Dense terminal numbering: position among unlabeled terminal tabs,
	// after the built-in Terminal when visible. Labeled terminals (command
	// apps) display their own label, so they don't consume a number.
	// Closing a terminal renumbers the ones after it.
	const terminalNumbers = new Map(
		validatedUserRightPanelTabs
			.filter((tab) => tab.kind === "terminal" && tab.label === undefined)
			.map(
				(tab, index) =>
					[tab.id, (hasBuiltInTerminal ? 1 : 0) + index + 1] as const,
			),
	);
	const sidebarTabConfigs = [
		...builtInSidebarTabConfigs,
		// Only unlabeled terminal tabs fall through to the numbered label;
		// every other tab kind has a required label.
		...validatedUserRightPanelTabs.map((tab) => {
			const terminalNumber = terminalNumbers.get(tab.id);
			return {
				id: tab.id,
				label:
					tab.label ??
					(terminalNumber === 1 ? "Terminal" : `Terminal ${terminalNumber}`),
			};
		}),
	];
	const sidebarTabIds = sidebarTabConfigs.map((tab) => tab.id);
	const effectiveSidebarTabId = getEffectiveTabId(
		sidebarTabIds,
		sidebarTabId,
		availableDesktopChatId,
	);

	const activateRightPanelTab = (tabId: string) => {
		onSetShowSidebarPanel(true);
		setPendingTabId(null);
		setSidebarTabId(tabId);
	};

	// Ignore late readiness from a tab the user already navigated past.
	const handleTerminalTabReady = (tabId: string) => {
		if (pendingTabId !== tabId) {
			return;
		}
		setPendingTabId(null);
		setSidebarTabId(tabId);
	};

	const handleActiveTabChange = (tabId: string) => {
		setPendingTabId(null);
		setSidebarTabId(tabId);
	};

	const startPendingTab = (tabId: string) => {
		onSetShowSidebarPanel(true);
		setPendingTabId(tabId);
	};

	const createUserRightPanelTabId = (
		kind: UserRightPanelTab["kind"],
	): string => {
		return `${kind}-${uuidv4()}`;
	};

	const handleAddTerminalTab = () => {
		if (!workspace || !workspaceAgent) {
			return;
		}
		// Reopen the built-in Terminal instead of creating Terminal 2 with no Terminal 1.
		if (defaultTerminalHidden) {
			setDefaultTerminalHiddenState(false);
			startPendingTab("terminal");
			return;
		}
		const tabId = createUserRightPanelTabId("terminal");
		setUserRightPanelTabsState((currentTabs) => [
			...currentTabs,
			{
				id: tabId,
				kind: "terminal",
				reconnectionToken: uuidv4(),
			},
		]);
		startPendingTab(tabId);
	};

	const handleOpenWorkspaceAppTab = (app: TypesGen.WorkspaceApp) => {
		if (!workspaceAgent) {
			return;
		}
		const existingTab = validatedUserRightPanelTabs.find(
			(tab) =>
				tab.kind === "workspace_app" &&
				tab.agentId === workspaceAgent.id &&
				tab.appId === app.id,
		);
		if (existingTab) {
			activateRightPanelTab(existingTab.id);
			return;
		}
		const tab: UserRightPanelTab = {
			id: createUserRightPanelTabId("workspace_app"),
			kind: "workspace_app",
			label: app.display_name ?? app.slug,
			agentId: workspaceAgent.id,
			appId: app.id,
		};
		setUserRightPanelTabsState((currentTabs) => [...currentTabs, tab]);
		activateRightPanelTab(tab.id);
	};

	const handleOpenCommandAppTab = (app: TypesGen.WorkspaceApp) => {
		if (!workspace || !workspaceAgent || !app.command) {
			return;
		}
		const existingTab = validatedUserRightPanelTabs.find(
			(tab) => tab.kind === "terminal" && tab.sourceAppId === app.id,
		);
		if (existingTab) {
			activateRightPanelTab(existingTab.id);
			return;
		}
		const tab: UserRightPanelTab = {
			id: createUserRightPanelTabId("terminal"),
			kind: "terminal",
			label: app.display_name ?? app.slug,
			reconnectionToken: uuidv4(),
			initialCommand: app.command,
			sourceAppId: app.id,
		};
		setUserRightPanelTabsState((currentTabs) => [...currentTabs, tab]);
		startPendingTab(tab.id);
	};

	const handleOpenPortTab = (selection: PortSelection) => {
		if (!workspaceAgent) {
			return;
		}
		const existingTab = validatedUserRightPanelTabs.find(
			(tab) =>
				tab.kind === "port" &&
				tab.agentId === workspaceAgent.id &&
				tab.port === selection.port &&
				tab.protocol === selection.protocol,
		);
		if (existingTab) {
			activateRightPanelTab(existingTab.id);
			return;
		}
		const tab: UserRightPanelTab = {
			id: createUserRightPanelTabId("port"),
			kind: "port",
			label: selection.label,
			agentId: workspaceAgent.id,
			port: selection.port,
			protocol: selection.protocol,
		};
		setUserRightPanelTabsState((currentTabs) => [...currentTabs, tab]);
		activateRightPanelTab(tab.id);
	};

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
						isGitStatusLoading={
							workspaceAgent?.status === "connected" &&
							!gitWatcher.hasReceivedChanges
						}
						onRefresh={handleRefresh}
						onCommit={handleCommit}
						isExpanded={visualExpanded}
						remoteDiffStats={diffStatusData}
						chatInputRef={editing.chatInputRef}
					/>
				);
			case "desktop":
				return availableDesktopChatId ? (
					<DesktopPanel
						chatId={availableDesktopChatId}
						isVisible={effectiveSidebarTabId === "desktop"}
					/>
				) : null;
			case "terminal":
				return workspace && workspaceAgent ? (
					<TerminalPanel
						chatId={agentId}
						isHot={
							shouldShowSidebar &&
							(effectiveSidebarTabId === "terminal" ||
								pendingTabId === "terminal")
						}
						autoFocus={
							shouldShowSidebar && effectiveSidebarTabId === "terminal"
						}
						onReady={() => handleTerminalTabReady("terminal")}
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
			default: {
				const userTab = validatedUserRightPanelTabs.find(
					(tab) => tab.id === tabId,
				);
				return userTab ? (
					<UserTabContent
						tab={userTab}
						chatId={agentId}
						workspace={workspace}
						workspaceAgent={workspaceAgent}
						wildcardHostname={wildcardHostname}
						sidebarVisible={shouldShowSidebar}
						isActive={effectiveSidebarTabId === userTab.id}
						isPending={pendingTabId === userTab.id}
						onTerminalReady={handleTerminalTabReady}
					/>
				) : null;
			}
		}
	};

	const handleCloseTab = (tabId: string) => {
		setPendingTabId((currentTabId) =>
			currentTabId === tabId ? null : currentTabId,
		);
		const remainingTabIds = sidebarTabIds.filter((id) => id !== tabId);
		const closedTabIndex = sidebarTabIds.indexOf(tabId);

		if (tabId === "terminal") {
			setDefaultTerminalHiddenState(true);
		} else {
			setUserRightPanelTabsState((currentTabs) =>
				currentTabs.filter((tab) => tab.id !== tabId),
			);
		}

		if (effectiveSidebarTabId !== tabId) {
			return;
		}
		const nextActiveTabId =
			remainingTabIds[Math.min(closedTabIndex, remainingTabIds.length - 1)];
		if (nextActiveTabId) {
			setSidebarTabId(nextActiveTabId);
		}
	};

	const sidebarTabs = sidebarTabConfigs.map((tab) => {
		const isCloseable =
			tab.id === "terminal" ||
			validatedUserRightPanelTabs.some((userTab) => userTab.id === tab.id);
		return {
			id: tab.id,
			label: tab.label,
			content: renderTabContent(tab.id),
			onClose: isCloseable ? () => handleCloseTab(tab.id) : undefined,
		};
	});

	const isEditing =
		editing.editingMessageId !== null ||
		editing.editingQueuedMessageID !== null;

	const chatOwnerUsername = chatOwner?.username?.trim();
	const chatOwnerLabel =
		chatOwner?.name?.trim() ||
		(chatOwnerUsername ? `@${chatOwnerUsername}` : "another user");
	const isOtherUserReadOnly = !isArchived && chatOwner !== undefined;
	const chatOwnerWarning = isOtherUserReadOnly
		? `This chat is owned by ${chatOwnerLabel}. It is read-only.`
		: undefined;

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
						"relative flex min-h-0 min-w-0 flex-1 sm:[--agents-chat-panel-min-width:360px]",
						shouldShowSidebar && !visualExpanded && "flex-row",
					)}
				>
					{titleElement}
					<div
						data-testid="agents-chat-panel"
						className={cn(
							"relative flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden sm:min-w-[var(--agents-chat-panel-min-width,0px)]",
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
								onPinAgent={handlePinAgentAction}
								onUnpinAgent={handleUnpinAgentAction}
								onOpenRenameDialog={handleOpenRenameDialogAction}
								isPinned={isPinned}
								isChildChat={isChildChat}
								isArchiving={isArchivingThisChat}
								hasWorkspace={Boolean(workspace)}
								isArchived={isArchived}
								diffStatusData={diffStatusData}
								isSharedChat={isSharedChat}
								isSidebarCollapsed={isSidebarCollapsed}
								onToggleSidebarCollapsed={onToggleSidebarCollapsed}
								renderChatSharingContent={
									canOpenChatSharing
										? (open) => (
												<ChatSharingPopoverContent
													chatId={agentId}
													organizationId={organizationId}
													open={open}
												/>
											)
										: undefined
								}
							/>
							{chatOwnerWarning && (
								<div
									role="status"
									aria-live="polite"
									className="flex shrink-0 items-center gap-2 border-b border-border-warning bg-surface-orange px-4 py-2 text-xs text-content-primary"
								>
									<TriangleAlertIcon className="size-4 shrink-0 text-content-warning" />
									{chatOwnerWarning}
								</div>
							)}
							{isArchived && (
								<div className="flex shrink-0 items-center gap-2 border-b border-border-default bg-surface-secondary px-4 py-2 text-xs text-content-secondary">
									<ArchiveIcon className="size-4 shrink-0" />
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
							<div className="px-4" data-chat-scroll-content>
								<ChatPageTimeline
									store={store}
									persistedError={persistedError}
									onEditUserMessage={
										isOtherUserReadOnly
											? undefined
											: editing.handleEditUserMessage
									}
									editingMessageId={editing.editingMessageId}
									urlTransform={urlTransform}
									mcpServers={mcpServers}
									onImplementPlan={
										isOtherUserReadOnly ? undefined : onImplementPlan
									}
									onSendAskUserQuestionResponse={
										isOtherUserReadOnly
											? undefined
											: canSendAskUserQuestionResponse
									}
								/>
							</div>
						</ChatScrollContainer>
						<div className="shrink-0 overflow-y-auto px-4 pb-3 md:pb-0 [scrollbar-gutter:stable] [scrollbar-width:thin]">
							<ChatPageInput
								organizationId={organizationId}
								sendShortcut={sendShortcut}
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
								canConfigureAgentSetup={canConfigureAgentSetup}
								providerCount={providerCount}
								modelCount={modelCount}
								unsupportedProviderNames={unsupportedProviderNames}
								aiGatewayDisabled={aiGatewayDisabled}
								selectedModel={effectiveSelectedModel}
								onModelChange={setSelectedModel}
								modelOptions={modelOptions}
								modelSelectorPlaceholder={modelSelectorPlaceholder}
								modelSelectorHelp={modelSelectorHelp}
								planModeEnabled={planModeEnabled}
								onPlanModeToggle={onPlanModeToggle}
								isModelCatalogLoading={isModelCatalogLoading}
								workspaceOptions={workspaceOptions}
								chatOrganizationId={organizationId}
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
								editingFileBlocks={editing.editingFileBlocks}
								mcpServers={mcpServers}
								selectedMCPServerIds={selectedMCPServerIds}
								onMCPSelectionChange={onMCPSelectionChange}
								onMCPAuthComplete={onMCPAuthComplete}
								chatContext={chatContext}
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
							onActiveTabChange={handleActiveTabChange}
							tabs={sidebarTabs}
							addTabControl={
								<RightPanelAddTabControl
									workspace={workspace}
									agent={workspaceAgent}
									host={wildcardHostname}
									isRunning={workspace?.latest_build.status === "running"}
									onNewTerminal={handleAddTerminalTab}
									onOpenWorkspaceApp={handleOpenWorkspaceAppTab}
									onOpenCommandApp={handleOpenCommandAppTab}
									onOpenPort={handleOpenPortTab}
								/>
							}
							onClose={() => onSetShowSidebarPanel(false)}
							isExpanded={visualExpanded}
							onToggleExpanded={() => setIsRightPanelExpanded((prev) => !prev)}
							isSidebarCollapsed={isSidebarCollapsed}
							onToggleSidebarCollapsed={onToggleSidebarCollapsed}
							chatTitle={chatTitle}
						/>
					</RightPanel>
				</div>
			</DesktopPanelContext>
		</ChatWorkspaceContext>
	);
};

interface AgentChatPageLoadingViewProps {
	sendShortcut: AgentChatSendShortcut;
	titleElement: React.ReactNode;
	inputRef: RefObject<ChatMessageInputRef | null>;
	initialValue: string;
	initialEditorState: string | undefined;
	remountKey: number;
	onContentChange: (
		content: string,
		serializedEditorState: string,
		hasFileReferences: boolean,
	) => void;
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
	sendShortcut,
	titleElement,
	inputRef,
	initialValue,
	initialEditorState,
	remountKey,
	onContentChange,
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
				"relative flex h-full min-h-0 min-w-0 flex-1 sm:[--agents-chat-panel-min-width:360px]",
				showRightPanel && "flex-row",
			)}
		>
			{titleElement}
			<div className="relative flex h-full min-h-0 min-w-0 flex-1 flex-col sm:min-w-[var(--agents-chat-panel-min-width,0px)]">
				<ChatTopBar
					panel={{
						showSidebarPanel: false,
						onToggleSidebar: () => {},
					}}
					onArchiveAgent={() => {}}
					onUnarchiveAgent={() => {}}
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
						sendShortcut={sendShortcut}
						inputRef={inputRef}
						initialValue={initialValue}
						initialEditorState={initialEditorState}
						remountKey={remountKey}
						onContentChange={onContentChange}
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
						canConfigureAgentSetup={false}
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
