import { cn } from "cn";
import { ArchiveIcon, TriangleAlertIcon } from "lucide-react";
import {
	type FC,
	type ReactNode,
	type RefObject,
	useEffect,
	useState,
} from "react";
import { useQueryClient } from "react-query";
import type { UrlTransform } from "streamdown";
import { invalidateChatDiffContents } from "#/api/queries/chats";
import type * as TypesGen from "#/api/typesGenerated";
import type { ChatMessagePart } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { useProxy } from "#/contexts/ProxyContext";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import type { ModelSelectorOption } from "#/modules/aiModels/ModelSelector";
import { getAgentBrowserApp } from "#/modules/apps/apps";
import { WorkspaceAppFrame } from "#/modules/apps/WorkspaceAppFrame";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { generateConnectionSessionId, generateUUID } from "#/utils/random";
import {
	AgentChatInput,
	type ChatMessageInputRef,
} from "./components/AgentChatInput";
import {
	ChatConversationSkeleton,
	RightPanelSkeleton,
} from "./components/AgentsSkeletons";
import type { ChatDetailError } from "./components/ChatConversation/chatError";
import {
	selectChatStatus,
	useChatSelector,
	type useChatStore,
} from "./components/ChatConversation/chatStore";

import { QueuedForCapacityCallout } from "./components/ChatConversation/QueuedForCapacityCallout";
import { DesktopPanelContext } from "./components/ChatElements/tools/DesktopPanelContext";
import type { PendingAttachment } from "./components/ChatPageContent";
import { ChatPageInput, ChatPageTimeline } from "./components/ChatPageContent";
import { ChatSummaryPanel } from "./components/ChatSummaryPanel";
import { getEffectiveTabId } from "./components/ChatsSidebar/tabs/getEffectiveTabId";
import { SidebarTabView } from "./components/ChatsSidebar/tabs/SidebarTabView";
import { ChatTopBar } from "./components/ChatTopBar";
import { countGitViews, GitPanel } from "./components/GitPanel/GitPanel";
import { DebugPanel } from "./components/RightPanel/DebugPanel/DebugPanel";
import { DesktopPanel } from "./components/RightPanel/DesktopPanel";
import { RightPanel } from "./components/RightPanel/RightPanel";
import {
	type TerminalChip,
	TerminalTabPanel,
} from "./components/RightPanel/TerminalTabPanel";
import {
	hasWorkspaceTabContent,
	WorkspaceTabPanel,
} from "./components/RightPanel/WorkspaceTabPanel";
import { getWorkspaceStatus, StatusIcon } from "./components/StatusIcon";
import { ChatWorkspaceContext } from "./context/ChatWorkspaceContext";
import { TerminalClientSessionContext } from "./context/TerminalClientSessionContext";
import { chatWidthClass, useChatFullWidth } from "./hooks/useChatFullWidth";
import { parsePullRequestUrl } from "./utils/pullRequest";
import {
	type ActiveSubTabIds,
	getPersistedActiveSubTabIds,
	getPersistedDefaultTerminalHidden,
	getPersistedRightPanelTabs,
	savePersistedActiveSubTabIds,
	savePersistedDefaultTerminalHidden,
	savePersistedRightPanelTabs,
} from "./utils/rightPanelTabStorage";
import {
	isTerminalRightPanelTab,
	isWorkspacePreviewRightPanelTab,
	type PortSelection,
	type RightPanelGroupTabId,
	type RightPanelTabId,
	resolveRightPanelTabId,
	type UserRightPanelTab,
	validateUserRightPanelTabs,
} from "./utils/rightPanelTabs";
import {
	getPersistedSidebarTabId,
	savePersistedSidebarTabId,
} from "./utils/sidebarTabStorage";

type ChatStoreHandle = ReturnType<typeof useChatStore>["store"];

type EditingState = {
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
	handleSendFromInput: (
		message: string,
		attachments?: readonly PendingAttachment[],
	) => void;
	handleContentChange: (
		content: string,
		serializedEditorState: string,
		hasFileReferences: boolean,
	) => void;
};

type AgentChatPageViewProps = {
	chat: TypesGen.Chat;
	persistedError: ChatDetailError | undefined;
	workspaceAgent?: TypesGen.WorkspaceAgent;
	workspace?: TypesGen.Workspace;

	// Store handle.
	store: ChatStoreHandle;
	/** Messages as first loaded; read once at mount for the initial anchor. */
	initialMessages: readonly TypesGen.ChatMessage[];

	// Editing state.
	editing: EditingState;

	// Model/input configuration.
	effectiveSelectedModel: string;
	setSelectedModel: (model: string) => void;
	modelOptions: readonly ModelSelectorOption[];
	models: readonly TypesGen.ChatModel[] | undefined;
	modelSelectorPlaceholder: string;
	modelSelectorHelp?: ReactNode;
	modelCatalogError?: unknown;
	unavailableModelNotice?: string;
	reasoningEffort?: string;
	onReasoningEffortChange?: (value: string) => void;
	canConfigureAgentSetup: boolean;
	providerCount?: number;
	modelCount?: number;
	unsupportedProviderNames?: readonly string[];
	aiGatewayDisabled?: boolean;
	hasModelOptions: boolean;
	isModelCatalogLoading?: boolean;
	onPlanModeToggle?: (enabled: boolean) => void;
	isInputDisabled: boolean;
	isSubmissionPending: boolean;
	isInterruptPending: boolean;
	onWorkspaceChange?: (workspaceId: string | null) => void;
	isWorkspaceLoading?: boolean;

	// Right panel state (owned by the parent so loading and
	// loaded views share the same layout).
	showSidebarPanel: boolean;
	onSetShowSidebarPanel: (next: boolean) => void;

	// Sidebar content data.
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

	// Pagination for loading older messages.
	hasMoreMessages: boolean;
	isFetchingMoreMessages: boolean;
	isHydratingMessages: boolean;
	hasFetchMoreError: boolean;
	onFetchMoreMessages: () => Promise<unknown>;

	urlTransform?: UrlTransform;

	// MCP server state.
	mcpServers: readonly TypesGen.MCPServerConfig[];
	selectedMCPServerIds: readonly string[];
	onMCPSelectionChange: (ids: string[]) => void;
	onMCPAuthComplete: (serverId: string) => void;

	// Desktop chat ID (optional).
	desktopChatId?: string;
};

export const AgentChatPageView: FC<AgentChatPageViewProps> = ({
	chat,
	persistedError,
	workspaceAgent,
	workspace,
	store,
	initialMessages,
	editing,
	effectiveSelectedModel,
	setSelectedModel,
	modelOptions,
	models,
	modelSelectorPlaceholder,
	modelSelectorHelp,
	modelCatalogError,
	unavailableModelNotice,
	reasoningEffort,
	onReasoningEffortChange,
	canConfigureAgentSetup,
	providerCount,
	modelCount,
	unsupportedProviderNames,
	aiGatewayDisabled,
	hasModelOptions,
	isModelCatalogLoading = false,
	onPlanModeToggle,
	isInputDisabled,
	isSubmissionPending,
	isInterruptPending,
	onWorkspaceChange,
	isWorkspaceLoading = false,
	showSidebarPanel,
	onSetShowSidebarPanel,
	debugLoggingEnabled,
	gitWatcher,
	sshCommand,
	handleCommit,
	handleInterrupt,
	handleDeleteQueuedMessage,
	handlePromoteQueuedMessage,
	onImplementPlan,
	onSendAskUserQuestionResponse,
	hasMoreMessages,
	isFetchingMoreMessages,
	isHydratingMessages,
	hasFetchMoreError,
	onFetchMoreMessages,
	urlTransform,
	mcpServers,
	selectedMCPServerIds,
	onMCPSelectionChange,
	onMCPAuthComplete,
	desktopChatId,
}) => {
	const queryClient = useQueryClient();
	const { proxy } = useProxy();
	const { entitlements } = useDashboard();
	const { permissions, user: currentUser } = useAuthenticated();
	const wildcardHostname = proxy.preferredWildcardHostname;
	const agentId = chat.id;
	const organizationId = chat.organization_id;
	const isArchived = chat.archived;
	const liveChatStatus =
		useChatSelector(store, selectChatStatus) ?? chat.status;
	const parsedPrNumber = Number(
		parsePullRequestUrl(chat.diff_status?.url)?.number,
	);
	const prNumber = chat.diff_status?.pr_number ?? (parsedPrNumber || undefined);

	const canSubmitChatTurn = !isInputDisabled && !isSubmissionPending;

	// Wrap the git watcher refresh to also invalidate the cached
	// remote/PR diff contents so the panel re-fetches from GitHub.
	const handleRefresh = () => {
		const sent = gitWatcher.refresh();
		if (sent && agentId) {
			void invalidateChatDiffContents(queryClient, agentId);
		}
		return sent;
	};

	const [isRightPanelExpanded, setIsRightPanelExpanded] = useState(false);
	// The turn already running when the page loaded must not anchor the
	// scroller, even if its prompt is older than the newest messages page and
	// only renders after the user pages up. Message ids are allocated from one
	// sequence, so every user row created after mount (sends, queue
	// promotions, edit re-sends) has an id above the initial maximum.
	const [initialActiveTurnMaxMessageId] = useState<number | undefined>(() =>
		chat.status === "running" || chat.status === "interrupting"
			? (initialMessages.at(-1)?.id ?? -1)
			: undefined,
	);
	const [dragVisualExpanded, setDragVisualExpanded] = useState<boolean | null>(
		null,
	);
	// Expansion must never outlive the panel: when narrow-viewport
	// suppression or an explicit close hides the panel, gate expansion
	// off (rather than resetting it) so it is restored with the panel.
	const visualExpanded =
		showSidebarPanel && (dragVisualExpanded ?? isRightPanelExpanded);

	const [sidebarTabId, setSidebarTabIdState] = useState<string | null>(() =>
		resolveRightPanelTabId(getPersistedSidebarTabId(agentId)),
	);
	const [userRightPanelTabs, setUserRightPanelTabsState] = useState<
		UserRightPanelTab[]
	>(() => getPersistedRightPanelTabs(agentId));
	const [defaultTerminalHidden, setDefaultTerminalHiddenState] =
		useState<boolean>(() => getPersistedDefaultTerminalHidden(agentId));
	const [activeSubTabIds, setActiveSubTabIdsState] = useState<ActiveSubTabIds>(
		() => getPersistedActiveSubTabIds(agentId),
	);
	const [pendingTerminalId, setPendingTerminalId] = useState<string | null>(
		null,
	);
	// One client session ID per page visit, shared by every terminal in this
	// chat. It regenerates when this view remounts (switching chats or
	// reloading), independent of any terminal's reconnection token.
	const [clientSessionId] = useState(generateConnectionSessionId);

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

	useEffect(() => {
		if (!isArchived) {
			savePersistedActiveSubTabIds(agentId, activeSubTabIds);
		}
	}, [agentId, isArchived, activeSubTabIds]);

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
	// offer the tab on that same condition to avoid selecting "desktop"
	// when no desktop panel is rendered.
	const availableDesktopChatId =
		workspace && workspaceAgent ? desktopChatId : undefined;

	const availableBrowserApp = workspace
		? getAgentBrowserApp(workspaceAgent)
		: undefined;

	const hasWorkspaceAgent =
		workspace !== undefined && workspaceAgent !== undefined;
	const isWorkspaceRunning = workspace?.latest_build.status === "running";
	const hasWorkspaceTab =
		hasWorkspaceAgent &&
		hasWorkspaceTabContent(workspaceAgent, wildcardHostname);

	const validatedUserRightPanelTabs = validateUserRightPanelTabs(
		userRightPanelTabs,
		{ workspace, workspaceAgent, wildcardHostname },
	);
	const userTerminalTabs = validatedUserRightPanelTabs.filter(
		isTerminalRightPanelTab,
	);
	const previewTabs = validatedUserRightPanelTabs.filter(
		isWorkspacePreviewRightPanelTab,
	);

	// Dense terminal numbering: the built-in terminal (reconnect token = chat
	// ID) is first when shown, then each unlabeled terminal in order. Labeled
	// terminals (command apps) display their own label and do not consume a
	// number. Closing a terminal renumbers the ones after it.
	let terminalNumber = 0;
	const terminalChips: TerminalChip[] = [];
	if (hasWorkspaceAgent && !defaultTerminalHidden) {
		terminalNumber += 1;
		terminalChips.push({
			id: "terminal",
			label: `Terminal ${terminalNumber}`,
			reconnectionToken: agentId,
		});
	}
	for (const tab of userTerminalTabs) {
		if (tab.label === undefined) {
			terminalNumber += 1;
		}
		terminalChips.push({
			id: tab.id,
			label: tab.label ?? `Terminal ${terminalNumber}`,
			reconnectionToken: tab.reconnectionToken,
			initialCommand: tab.initialCommand,
		});
	}
	const terminalChipIds = terminalChips.map((chip) => chip.id);
	const effectiveTerminalId = getEffectiveTabId(
		terminalChipIds,
		activeSubTabIds.terminal ?? null,
	);
	const previewIds = previewTabs.map((tab) => tab.id);
	const effectivePreviewId = getEffectiveTabId(
		previewIds,
		activeSubTabIds.workspace ?? null,
	);

	const prTab = prNumber && agentId ? { prNumber, chatId: agentId } : undefined;
	const gitViewCount = countGitViews({
		prTab,
		repositories: gitWatcher.repositories,
		remoteDiffStats: chat.diff_status,
		everDirty: gitWatcher.everDirty,
	});

	// A badge only appears when there is something to choose between.
	const countBadge = (count: number) => (count > 1 ? count : undefined);

	// Single source of truth for available tabs and their order. Tabs appear
	// when their content is available and are never closed by the user.
	const sidebarTabConfigs: {
		id: RightPanelTabId;
		label: string;
		badge?: number;
	}[] = [
		{ id: "summary", label: "Summary" },
		{ id: "git", label: "Git", badge: countBadge(gitViewCount) },
		...(hasWorkspaceAgent
			? [
					{
						id: "terminal" as const,
						label: "Terminal",
						badge: countBadge(terminalChips.length),
					},
				]
			: []),
		...(availableBrowserApp
			? [{ id: "browser" as const, label: "Browser" }]
			: []),
		...(availableDesktopChatId
			? [{ id: "desktop" as const, label: "Desktop" }]
			: []),
		...(hasWorkspaceTab
			? [
					{
						id: "workspace" as const,
						label: "Workspace",
						badge: countBadge(previewTabs.length),
					},
				]
			: []),
		...(debugLoggingEnabled ? [{ id: "debug" as const, label: "Debug" }] : []),
	];
	const sidebarTabIds = sidebarTabConfigs.map((tab) => tab.id);
	const effectiveSidebarTabId = getEffectiveTabId(sidebarTabIds, sidebarTabId);

	const activateRightPanelTab = (tabId: RightPanelTabId) => {
		onSetShowSidebarPanel(true);
		setSidebarTabId(tabId);
	};

	const setActiveSubTabId = (group: RightPanelGroupTabId, tabId: string) => {
		setActiveSubTabIdsState((current) => ({ ...current, [group]: tabId }));
	};

	const desktopPanelCtx = {
		desktopChatId,
		// Only offer the action when the panel can render, which keeps a tool
		// action from selecting a Desktop tab that the tab list omits.
		onOpenDesktop: availableDesktopChatId
			? () => activateRightPanelTab("desktop")
			: undefined,
	};

	const activateTerminalChip = (terminalId: string) => {
		setPendingTerminalId(null);
		setActiveSubTabId("terminal", terminalId);
	};

	// Ignore late readiness from a terminal the user already navigated past.
	const handleTerminalReady = (terminalId: string) => {
		if (pendingTerminalId !== terminalId) {
			return;
		}
		setPendingTerminalId(null);
		setActiveSubTabId("terminal", terminalId);
	};

	// A new terminal is shown once it reports ready so the chip does not
	// switch to a blank canvas mid-connect.
	const startPendingTerminal = (terminalId: string) => {
		activateRightPanelTab("terminal");
		setPendingTerminalId(terminalId);
	};

	const createUserRightPanelTabId = (
		kind: UserRightPanelTab["kind"],
	): string => {
		return `${kind}-${generateUUID()}`;
	};

	const handleAddTerminal = () => {
		if (!hasWorkspaceAgent) {
			return;
		}
		// Reopen the built-in Terminal instead of creating Terminal 2 with no Terminal 1.
		if (defaultTerminalHidden) {
			setDefaultTerminalHiddenState(false);
			startPendingTerminal("terminal");
			return;
		}
		const tabId = createUserRightPanelTabId("terminal");
		setUserRightPanelTabsState((currentTabs) => [
			...currentTabs,
			{
				id: tabId,
				kind: "terminal",
				reconnectionToken: generateUUID(),
			},
		]);
		startPendingTerminal(tabId);
	};

	const openPreview = (previewId: string) => {
		activateRightPanelTab("workspace");
		setActiveSubTabId("workspace", previewId);
	};

	const handleOpenWorkspaceAppTab = (app: TypesGen.WorkspaceApp) => {
		if (!workspaceAgent) {
			return;
		}
		const existingTab = previewTabs.find(
			(tab) =>
				tab.kind === "workspace_app" &&
				tab.agentId === workspaceAgent.id &&
				tab.appId === app.id,
		);
		if (existingTab) {
			openPreview(existingTab.id);
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
		openPreview(tab.id);
	};

	const handleOpenCommandAppTab = (app: TypesGen.WorkspaceApp) => {
		if (!hasWorkspaceAgent || !app.command) {
			return;
		}
		const existingTab = userTerminalTabs.find(
			(tab) => tab.sourceAppId === app.id,
		);
		if (existingTab) {
			activateRightPanelTab("terminal");
			activateTerminalChip(existingTab.id);
			return;
		}
		const tab: UserRightPanelTab = {
			id: createUserRightPanelTabId("terminal"),
			kind: "terminal",
			label: app.display_name ?? app.slug,
			reconnectionToken: generateUUID(),
			initialCommand: app.command,
			sourceAppId: app.id,
		};
		setUserRightPanelTabsState((currentTabs) => [...currentTabs, tab]);
		startPendingTerminal(tab.id);
	};

	const handleOpenPortTab = (selection: PortSelection) => {
		if (!workspaceAgent) {
			return;
		}
		const existingTab = previewTabs.find(
			(tab) =>
				tab.kind === "port" &&
				tab.agentId === workspaceAgent.id &&
				tab.port === selection.port &&
				tab.protocol === selection.protocol,
		);
		if (existingTab) {
			openPreview(existingTab.id);
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
		openPreview(tab.id);
	};

	// Closing the active chip selects its right-hand neighbor, or the new
	// last chip when the closed one was last.
	const selectNeighborAfterClose = (
		group: RightPanelGroupTabId,
		chipIds: readonly string[],
		activeId: string | null,
		closedId: string,
	) => {
		if (activeId !== closedId) {
			return;
		}
		const remainingIds = chipIds.filter((id) => id !== closedId);
		const closedIndex = chipIds.indexOf(closedId);
		const nextId = remainingIds[Math.min(closedIndex, remainingIds.length - 1)];
		if (nextId) {
			setActiveSubTabId(group, nextId);
		}
	};

	const handleCloseTerminal = (terminalId: string) => {
		setPendingTerminalId((current) =>
			current === terminalId ? null : current,
		);
		if (terminalId === "terminal") {
			setDefaultTerminalHiddenState(true);
		} else {
			setUserRightPanelTabsState((currentTabs) =>
				currentTabs.filter((tab) => tab.id !== terminalId),
			);
		}
		selectNeighborAfterClose(
			"terminal",
			terminalChipIds,
			effectiveTerminalId,
			terminalId,
		);
	};

	const handleClosePreview = (previewId: string) => {
		setUserRightPanelTabsState((currentTabs) =>
			currentTabs.filter((tab) => tab.id !== previewId),
		);
		selectNeighborAfterClose(
			"workspace",
			previewIds,
			effectivePreviewId,
			previewId,
		);
	};

	const renderTabContent = (tabId: RightPanelTabId): ReactNode => {
		const isTabActive = shouldShowSidebar && effectiveSidebarTabId === tabId;
		switch (tabId) {
			case "summary":
				return <ChatSummaryPanel chatId={agentId} isVisible={isTabActive} />;
			case "git":
				return (
					<GitPanel
						prTab={prTab}
						repositories={gitWatcher.repositories}
						everDirty={gitWatcher.everDirty}
						isGitStatusLoading={
							workspaceAgent?.status === "connected" &&
							!gitWatcher.hasReceivedChanges
						}
						onRefresh={handleRefresh}
						onCommit={handleCommit}
						isExpanded={visualExpanded}
						remoteDiffStats={chat.diff_status}
						chatInputRef={editing.chatInputRef}
					/>
				);
			case "browser":
				return workspace && workspaceAgent && availableBrowserApp ? (
					<WorkspaceAppFrame
						workspace={workspace}
						app={{ ...availableBrowserApp, agent: workspaceAgent }}
						active={effectiveSidebarTabId === "browser"}
					/>
				) : null;
			case "desktop":
				return availableDesktopChatId ? (
					<DesktopPanel
						chatId={availableDesktopChatId}
						isVisible={effectiveSidebarTabId === "desktop"}
					/>
				) : null;
			case "terminal":
				return workspace && workspaceAgent ? (
					<TerminalTabPanel
						chatId={agentId}
						workspace={workspace}
						workspaceAgent={workspaceAgent}
						terminals={terminalChips}
						activeTerminalId={effectiveTerminalId}
						pendingTerminalId={pendingTerminalId}
						isVisible={isTabActive}
						canCreateTerminal={isWorkspaceRunning}
						onActiveTerminalChange={activateTerminalChip}
						onCloseTerminal={handleCloseTerminal}
						onNewTerminal={handleAddTerminal}
						onTerminalReady={handleTerminalReady}
					/>
				) : null;
			case "workspace":
				return workspace && workspaceAgent ? (
					<WorkspaceTabPanel
						workspace={workspace}
						agent={workspaceAgent}
						host={wildcardHostname}
						isRunning={isWorkspaceRunning}
						previews={previewTabs}
						activePreviewId={effectivePreviewId}
						isVisible={isTabActive}
						onActivePreviewChange={(previewId) =>
							setActiveSubTabId("workspace", previewId)
						}
						onClosePreview={handleClosePreview}
						onOpenWorkspaceApp={handleOpenWorkspaceAppTab}
						onOpenCommandApp={handleOpenCommandAppTab}
						onOpenPort={handleOpenPortTab}
					/>
				) : null;
			case "debug":
				return <DebugPanel chatId={agentId} isVisible={isTabActive} />;
			default: {
				const _exhaustive: never = tabId;
				return _exhaustive;
			}
		}
	};

	const sidebarTabs = sidebarTabConfigs.map((tab) => ({
		id: tab.id,
		label: tab.label,
		badge: tab.badge,
		content: renderTabContent(tab.id),
	}));

	const isEditing = editing.editingMessageId !== null;

	const chatOwnerUsername = chat.owner_username?.trim();
	const chatOwnerLabel =
		chat.owner_name?.trim() ||
		(chatOwnerUsername ? `@${chatOwnerUsername}` : "another user");
	const isOtherUserReadOnly = !isArchived && currentUser.id !== chat.owner_id;
	const chatOwnerWarning = isOtherUserReadOnly
		? `This chat is owned by ${chatOwnerLabel}. It is read-only.`
		: undefined;

	const hasLicense = entitlements.has_license;
	const canManageLicenses = permissions.viewAllLicenses;
	const runtimeHours = entitlements.features.agent_runtime_hours;
	const agentHoursHardLimit =
		runtimeHours.enabled &&
		runtimeHours.hard_limit !== undefined &&
		runtimeHours.actual !== undefined &&
		runtimeHours.actual >= runtimeHours.hard_limit
			? runtimeHours.hard_limit
			: undefined;

	return (
		<TerminalClientSessionContext value={clientSessionId}>
			<ChatWorkspaceContext
				value={{ workspaceId: workspace?.id, buildId: chat.build_id }}
			>
				<DesktopPanelContext value={desktopPanelCtx}>
					<div
						className={cn(
							"relative flex h-full min-h-0 min-w-0 flex-1 sm:[--agents-chat-panel-min-width:360px]",
							shouldShowSidebar && !visualExpanded && "flex-row",
						)}
					>
						<div
							data-testid="agents-chat-panel"
							className={cn(
								"relative flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden sm:min-w-(--agents-chat-panel-min-width,0px)",
								visualExpanded && "hidden",
								shouldShowSidebar && "max-lg:hidden",
							)}
						>
							<div className="relative z-10 shrink-0 overflow-visible">
								{" "}
								<ChatTopBar
									chat={chat}
									liveChatStatus={liveChatStatus}
									panel={{
										showSidebarPanel,
										onToggleSidebar: () =>
											onSetShowSidebarPanel(!showSidebarPanel),
									}}
								/>
								{modelCatalogError != null && (
									<ErrorAlert error={modelCatalogError} />
								)}
								{unavailableModelNotice && (
									<div
										role="status"
										aria-label={unavailableModelNotice}
										aria-live="polite"
										className="flex shrink-0 items-center gap-2 border-b border-border-warning bg-surface-orange px-4 py-2 text-xs text-content-primary"
									>
										<TriangleAlertIcon className="size-4 shrink-0 text-content-warning" />
										{unavailableModelNotice}
									</div>
								)}
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
							<ChatPageTimeline
								key={agentId}
								organizationId={organizationId}
								store={store}
								chatFiles={chat.files}
								initialActiveTurnMaxMessageId={initialActiveTurnMaxMessageId}
								persistedError={persistedError}
								hasMoreMessages={hasMoreMessages}
								isFetchingMoreMessages={isFetchingMoreMessages}
								isHydratingMessages={isHydratingMessages}
								hasFetchMoreError={hasFetchMoreError}
								onFetchMoreMessages={onFetchMoreMessages}
								onEditUserMessage={
									isOtherUserReadOnly
										? undefined
										: editing.handleEditUserMessage
								}
								editingMessageId={editing.editingMessageId}
								urlTransform={urlTransform}
								mcpServers={mcpServers}
								onImplementPlan={
									isOtherUserReadOnly || !canSubmitChatTurn
										? undefined
										: onImplementPlan
								}
								onSendAskUserQuestionResponse={
									isOtherUserReadOnly || !canSubmitChatTurn
										? undefined
										: onSendAskUserQuestionResponse
								}
								footer={
									chat.queued_for_capacity ? (
										<QueuedForCapacityCallout
											hasLicense={hasLicense}
											canManageLicenses={canManageLicenses}
											agentHoursHardLimit={agentHoursHardLimit}
										/>
									) : undefined
								}
							/>
							{!isArchived && (
								<div className="shrink-0 overflow-y-auto px-4 pb-3 md:pb-0 scrollbar-gutter-stable scrollbar-thin">
									<ChatPageInput
										chat={chat}
										store={store}
										models={models}
										onSend={editing.handleSendFromInput}
										onDeleteQueuedMessage={handleDeleteQueuedMessage}
										onPromoteQueuedMessage={handlePromoteQueuedMessage}
										onInterrupt={handleInterrupt}
										isInputDisabled={isInputDisabled}
										isReadOnly={isOtherUserReadOnly}
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
										reasoningEffort={reasoningEffort}
										onReasoningEffortChange={onReasoningEffortChange}
										onPlanModeToggle={onPlanModeToggle}
										isModelCatalogLoading={isModelCatalogLoading}
										onWorkspaceChange={onWorkspaceChange}
										isWorkspaceLoading={isWorkspaceLoading}
										inputRef={editing.chatInputRef}
										initialValue={editing.editorInitialValue}
										initialEditorState={editing.initialEditorState}
										remountKey={editing.remountKey}
										onContentChange={editing.handleContentChange}
										isEditing={isEditing}
										onCancelHistoryEdit={editing.handleCancelHistoryEdit}
										editingFileBlocks={editing.editingFileBlocks}
										mcpServers={mcpServers}
										selectedMCPServerIds={selectedMCPServerIds}
										onMCPSelectionChange={onMCPSelectionChange}
										onMCPAuthComplete={onMCPAuthComplete}
										workspace={workspace}
										workspaceAgent={workspaceAgent}
										sshCommand={sshCommand}
										attachedWorkspace={attachedWorkspace}
										folder={preferredFolder}
									/>
								</div>
							)}
						</div>
						<RightPanel
							isOpen={shouldShowSidebar}
							isExpanded={showSidebarPanel && isRightPanelExpanded}
							onToggleExpanded={() => setIsRightPanelExpanded((prev) => !prev)}
							onClose={() => onSetShowSidebarPanel(false)}
							onVisualExpandedChange={setDragVisualExpanded}
						>
							<SidebarTabView
								effectiveTabId={effectiveSidebarTabId}
								onActiveTabChange={setSidebarTabId}
								tabs={sidebarTabs}
								onClose={() => onSetShowSidebarPanel(false)}
								isExpanded={visualExpanded}
								onToggleExpanded={() =>
									setIsRightPanelExpanded((prev) => !prev)
								}
								chatTitle={chat.title}
							/>
						</RightPanel>
					</div>
				</DesktopPanelContext>
			</ChatWorkspaceContext>
		</TerminalClientSessionContext>
	);
};

type AgentChatPageLoadingViewProps = {
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
	showRightPanel: boolean;
};

export const AgentChatPageLoadingView: FC<AgentChatPageLoadingViewProps> = ({
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
			<div className="relative flex h-full min-h-0 min-w-0 flex-1 flex-col sm:min-w-(--agents-chat-panel-min-width,0px)">
				<ChatTopBar
					panel={{
						showSidebarPanel: false,
						onToggleSidebar: () => {},
					}}
				/>
				<div className="min-h-0 flex-1 overflow-y-auto scrollbar-gutter-stable scrollbar-thin [scrollbar-color:hsl(var(--surface-quaternary))_transparent]">
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
				<div className="shrink-0 overflow-y-auto px-4 pb-3 md:pb-0 scrollbar-gutter-stable scrollbar-thin">
					<AgentChatInput
						onSend={() => {}}
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
				>
					<RightPanelSkeleton />
				</RightPanel>
			)}
		</div>
	);
};

export const AgentChatPageNotFoundView: FC = () => {
	return (
		<div className="flex h-full min-h-0 min-w-0 flex-1 flex-col">
			<ChatTopBar
				panel={{
					showSidebarPanel: false,
					onToggleSidebar: () => {},
				}}
			/>
			<div className="flex flex-1 items-center justify-center text-content-secondary">
				Chat not found
			</div>
		</div>
	);
};
