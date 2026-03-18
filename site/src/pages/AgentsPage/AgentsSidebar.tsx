import type {
	Chat,
	ChatDiffStatus,
	ChatModelConfig,
	ChatStatus,
} from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Avatar } from "components/Avatar/Avatar";
import type { ModelSelectorOption } from "components/ai-elements";
import { Button } from "components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "components/DropdownMenu/DropdownMenu";
import { ExternalImage } from "components/ExternalImage/ExternalImage";
import { CoderIcon } from "components/Icons/CoderIcon";
import { ScrollArea } from "components/ScrollArea/ScrollArea";
import { Skeleton } from "components/Skeleton/Skeleton";
import { Spinner } from "components/Spinner/Spinner";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { useAuthenticated } from "hooks";
import {
	AlertTriangleIcon,
	ArchiveIcon,
	ArchiveRestoreIcon,
	BarChart3Icon,
	BoxesIcon,
	CheckIcon,
	ChevronDownIcon,
	ChevronLeftIcon,
	ChevronRightIcon,
	EllipsisIcon,
	FilterIcon,
	GitMergeIcon,
	GitPullRequestArrowIcon,
	GitPullRequestClosedIcon,
	GitPullRequestDraftIcon,
	KeyRoundIcon,
	Loader2Icon,
	PanelLeftCloseIcon,
	PauseIcon,
	SettingsIcon,
	ShieldAlertIcon,
	ShieldIcon,
	SquarePenIcon,
	Trash2Icon,
	UserIcon,
} from "lucide-react";
import { UserDropdownContent } from "modules/dashboard/Navbar/UserDropdown/UserDropdownContent";
import { useDashboard } from "modules/dashboard/useDashboard";
import {
	createContext,
	type FC,
	memo,
	useCallback,
	useContext,
	useEffect,
	useMemo,
	useRef,
	useState,
} from "react";
import { Link, NavLink, useLocation, useParams } from "react-router";
import { cn } from "utils/cn";
import { shortRelativeTime } from "utils/time";
import { getTimeGroup, TIME_GROUPS } from "./timeGroups";

type SidebarView =
	| { panel: "chats" }
	| { panel: "settings"; section: string }
	| { panel: "analytics" };

/**
 * Derive the current sidebar view from the URL pathname.
 */
export function sidebarViewFromPath(pathname: string): SidebarView {
	if (pathname.startsWith("/agents/analytics")) {
		return { panel: "analytics" };
	}
	const settingsMatch = pathname.match(/^\/agents\/settings(?:\/([^/]+))?/);
	if (settingsMatch) {
		return { panel: "settings", section: settingsMatch[1] || "behavior" };
	}
	return { panel: "chats" };
}

interface AgentsSidebarProps {
	chats: readonly Chat[];
	chatErrorReasons: Record<string, string>;
	modelOptions: readonly ModelSelectorOption[];
	modelConfigs: readonly ChatModelConfig[];
	logoUrl?: string;
	onArchiveAgent: (chatId: string) => void;
	onUnarchiveAgent: (chatId: string) => void;
	onArchiveAndDeleteWorkspace: (chatId: string, workspaceId: string) => void;
	onBeforeNewAgent?: () => void;
	isCreating: boolean;
	isArchiving?: boolean;
	archivingChatId?: string | null;
	isLoading?: boolean;
	loadError?: unknown;
	onRetryLoad?: () => void;
	hasNextPage?: boolean;
	onLoadMore?: () => void;
	isFetchingNextPage?: boolean;
	archivedFilter: "active" | "archived";
	onArchivedFilterChange?: (filter: "active" | "archived") => void;
	onCollapse?: () => void;
	isAdmin?: boolean;
}

const statusConfig = {
	waiting: { icon: CheckIcon, className: "text-content-secondary" },
	pending: { icon: Loader2Icon, className: "text-content-link animate-spin" },
	running: { icon: Loader2Icon, className: "text-content-link animate-spin" },
	paused: { icon: PauseIcon, className: "text-content-warning" },
	error: { icon: AlertTriangleIcon, className: "text-content-destructive" },
	completed: { icon: CheckIcon, className: "text-content-secondary" },
} as const;

type ChatTree = {
	readonly rootIds: readonly string[];
	readonly childrenById: ReadonlyMap<string, readonly string[]>;
	readonly parentById: ReadonlyMap<string, string | undefined>;
};

const getStatusConfig = (status: ChatStatus) => {
	return statusConfig[status] ?? statusConfig.completed;
};

/**
 * Returns the icon and className to use for a PR state, or undefined
 * if there is no PR linked. Only overrides the icon when the chat
 * is not actively executing (pending/running/paused/error).
 */
const getPRIconConfig = (
	diffStatus: ChatDiffStatus | undefined,
): { icon: typeof CheckIcon; className: string } | undefined => {
	const state = diffStatus?.pull_request_state;
	if (!state) {
		return undefined;
	}
	if (state === "merged") {
		return { icon: GitMergeIcon, className: "text-git-merged-bright" };
	}
	if (state === "closed") {
		return {
			icon: GitPullRequestClosedIcon,
			className: "text-git-deleted-bright",
		};
	}
	// state === "open"
	if (diffStatus?.pull_request_draft) {
		return {
			icon: GitPullRequestDraftIcon,
			className: "text-content-secondary",
		};
	}
	return { icon: GitPullRequestArrowIcon, className: "text-git-added-bright" };
};

const asNonEmptyString = (value: unknown): string | undefined => {
	if (typeof value !== "string") {
		return undefined;
	}
	const trimmed = value.trim();
	return trimmed.length > 0 ? trimmed : undefined;
};

const getModelDisplayName = (
	lastModelConfigID: Chat["last_model_config_id"] | undefined,
	modelConfigs: readonly ChatModelConfig[],
	modelOptions: readonly ModelSelectorOption[],
) => {
	if (!lastModelConfigID) {
		return "Default model";
	}
	const modelConfig = modelConfigs.find(
		(config) => config.id === lastModelConfigID,
	);
	if (!modelConfig) {
		return "Default model";
	}
	const provider = modelConfig.provider.trim().toLowerCase();
	const model = modelConfig.model.trim();
	if (!provider || !model) {
		return modelConfig.display_name.trim() || "Default model";
	}

	// Try to find a matching option with a display name.
	const match = modelOptions.find(
		(opt) =>
			opt.id === `${provider}:${model}` ||
			(opt.provider === provider && opt.model === model),
	);
	if (match?.displayName) {
		return match.displayName;
	}

	if (modelConfig.display_name.trim()) {
		return modelConfig.display_name.trim();
	}

	return model;
};

const getChatDiffStatus = (chat: Chat): ChatDiffStatus | undefined => {
	return chat.diff_status;
};

const getParentChatID = (chat: Chat): string | undefined => {
	return asNonEmptyString(chat.parent_chat_id);
};

const getRootChatID = (chat: Chat): string | undefined => {
	return asNonEmptyString(chat.root_chat_id);
};

const buildChatTree = (chats: readonly Chat[]): ChatTree => {
	const orderById = new Map<string, number>();
	const chatById = new Map<string, Chat>();
	const parentById = new Map<string, string | undefined>();
	const childrenById = new Map<string, string[]>();

	for (const [index, chat] of chats.entries()) {
		orderById.set(chat.id, index);
		chatById.set(chat.id, chat);
		childrenById.set(chat.id, []);
	}

	for (const chat of chats) {
		let parentID = getParentChatID(chat);
		if (!parentID || parentID === chat.id || !chatById.has(parentID)) {
			parentID = undefined;
		}

		if (!parentID) {
			const rootID = getRootChatID(chat);
			if (rootID && rootID !== chat.id && chatById.has(rootID)) {
				parentID = rootID;
			}
		}

		parentById.set(chat.id, parentID);
		if (parentID) {
			childrenById.get(parentID)?.push(chat.id);
		}
	}

	for (const children of childrenById.values()) {
		children.sort((leftID, rightID) => {
			return (orderById.get(leftID) ?? 0) - (orderById.get(rightID) ?? 0);
		});
	}

	const rootIds = chats
		.map((chat) => chat.id)
		.filter((chatID) => !parentById.get(chatID));

	return {
		rootIds,
		childrenById,
		parentById,
	};
};

const collectVisibleChatIDs = ({
	chats,
	search,
	tree,
}: {
	readonly chats: readonly Chat[];
	readonly search: string;
	readonly tree: ChatTree;
}): Set<string> => {
	if (!search) {
		return new Set(chats.map((chat) => chat.id));
	}

	const matchedChatIDs = chats
		.filter((chat) => chat.title.toLowerCase().includes(search))
		.map((chat) => chat.id);
	if (matchedChatIDs.length === 0) {
		return new Set<string>();
	}

	const visible = new Set<string>();
	for (const matchedChatID of matchedChatIDs) {
		let parentCursor: string | undefined = matchedChatID;
		const seenParents = new Set<string>();
		while (parentCursor && !seenParents.has(parentCursor)) {
			seenParents.add(parentCursor);
			visible.add(parentCursor);
			parentCursor = tree.parentById.get(parentCursor);
		}

		const stack = [matchedChatID];
		const seenDescendants = new Set<string>();
		while (stack.length > 0) {
			const currentID = stack.pop();
			if (!currentID || seenDescendants.has(currentID)) {
				continue;
			}
			seenDescendants.add(currentID);
			visible.add(currentID);
			for (const childID of tree.childrenById.get(currentID) ?? []) {
				stack.push(childID);
			}
		}
	}

	return visible;
};

interface ChatTreeContextValue {
	readonly chatTree: ChatTree;
	readonly chatById: ReadonlyMap<string, Chat>;
	readonly visibleChatIDs: ReadonlySet<string>;
	readonly normalizedSearch: string;
	readonly expandedById: Record<string, boolean>;
	readonly modelOptions: readonly ModelSelectorOption[];
	readonly modelConfigs: readonly ChatModelConfig[];
	readonly chatErrorReasons: Record<string, string>;
	readonly isArchiving: boolean;
	readonly archivingChatId: string | null;
	readonly toggleExpanded: (chatID: string) => void;
	readonly onArchiveAgent: (chatId: string) => void;
	readonly onUnarchiveAgent: (chatId: string) => void;
	readonly onArchiveAndDeleteWorkspace: (
		chatId: string,
		workspaceId: string,
	) => void;
}

const ChatTreeContext = createContext<ChatTreeContextValue | null>(null);

function useChatTree(): ChatTreeContextValue {
	const ctx = useContext(ChatTreeContext);
	if (!ctx) {
		throw new Error("useChatTree must be used within ChatTreeContext.Provider");
	}
	return ctx;
}

interface ChatTreeNodeProps {
	readonly chat: Chat;
	readonly isChildNode: boolean;
}

const ChatTreeNode = memo<ChatTreeNodeProps>(({ chat, isChildNode }) => {
	const {
		chatTree,
		chatById,
		visibleChatIDs,
		normalizedSearch,
		expandedById,
		modelOptions,
		modelConfigs,
		chatErrorReasons,
		isArchiving,
		archivingChatId,
		toggleExpanded,
		onArchiveAgent,
		onUnarchiveAgent,
		onArchiveAndDeleteWorkspace,
	} = useChatTree();
	const chatID = chat.id;
	const childIDs = (chatTree.childrenById.get(chatID) ?? []).filter((childID) =>
		visibleChatIDs.has(childID),
	);
	const hasChildren = childIDs.length > 0;
	const isDelegated = Boolean(getParentChatID(chat));
	const isDelegatedExecuting =
		isDelegated && (chat.status === "pending" || chat.status === "running");
	const modelName = getModelDisplayName(
		chat.last_model_config_id,
		modelConfigs,
		modelOptions,
	);
	const errorReason =
		chat.status === "error"
			? chatErrorReasons[chat.id] || chat.last_error || undefined
			: undefined;
	const subtitle = errorReason || modelName;
	const diffStatus = getChatDiffStatus(chat);
	const baseConfig = getStatusConfig(chat.status);
	const prConfig =
		chat.status === "waiting" || chat.status === "completed"
			? getPRIconConfig(diffStatus)
			: undefined;
	const config = prConfig ?? baseConfig;
	const StatusIcon = config.icon;
	const hasLinkedDiffStatus = Boolean(diffStatus?.url);
	const changedFiles = diffStatus?.changed_files ?? 0;
	const additions = diffStatus?.additions ?? 0;
	const deletions = diffStatus?.deletions ?? 0;
	const hasLineStats = additions > 0 || deletions > 0 || changedFiles > 0;
	const filesChangedLabel = `${changedFiles} ${
		changedFiles === 1 ? "file" : "files"
	}`;
	const workspaceId = chat.workspace_id;
	const isArchivingThisChat = isArchiving && archivingChatId === chat.id;
	const isExpanded = normalizedSearch ? true : (expandedById[chatID] ?? false);

	return (
		<div className="flex min-w-0 flex-col">
			<div
				data-testid={`agents-tree-node-${chat.id}`}
				className={cn(
					"group relative flex min-w-0 items-start gap-1.5 rounded-md px-1 text-content-secondary",
					"transition-none [@media(hover:hover)]:hover:bg-surface-tertiary/50 [@media(hover:hover)]:hover:text-content-primary has-[[data-state=open]]:bg-surface-tertiary",
					"has-[[aria-current=page]]:bg-surface-quaternary/25 has-[[aria-current=page]]:text-content-primary [@media(hover:hover)]:has-[[aria-current=page]]:hover:bg-surface-quaternary/50",
					isChildNode &&
						"before:absolute before:-left-2.5 before:top-[17px] before:h-px before:w-2.5 before:bg-border-default/70",
				)}
			>
				<div
					className={cn(
						"group/icon relative mt-1.5 h-5 w-5 shrink-0",
						hasChildren && "cursor-pointer",
					)}
				>
					<div
						className={cn(
							"flex h-5 w-5 items-center justify-center rounded-md",
							hasChildren && "[@media(hover:hover)]:group-hover/icon:invisible",
						)}
					>
						<StatusIcon
							data-testid={
								isDelegatedExecuting
									? `agents-tree-executing-${chat.id}`
									: undefined
							}
							className={cn("h-3.5 w-3.5 shrink-0", config.className)}
						/>
					</div>
					{hasChildren && (
						<Button
							variant="subtle"
							size="icon"
							onClick={() => toggleExpanded(chatID)}
							className={cn(
								"absolute inset-0 invisible flex h-5 w-5 min-w-0 items-center justify-center rounded-md p-0 text-content-secondary/60 hover:text-content-primary [&>svg]:size-3.5",
								"[@media(hover:hover)]:group-hover/icon:visible",
							)}
							data-testid={`agents-tree-toggle-${chat.id}`}
							aria-label={isExpanded ? "Collapse" : "Expand"}
							aria-expanded={isExpanded}
						>
							{isExpanded ? <ChevronDownIcon /> : <ChevronRightIcon />}
						</Button>
					)}
				</div>
				<NavLink
					to={`/agents/${chat.id}`}
					className="flex min-h-0 min-w-0 flex-1 items-start gap-2 rounded-[inherit] py-1 pr-0.5 text-inherit no-underline"
				>
					{({ isActive }) => (
						<>
							<div className="min-w-0 flex-1 overflow-hidden text-left">
								<div className="flex min-w-0 items-center gap-1.5 overflow-hidden">
									<span
										className={cn(
											"block flex-1 truncate text-[13px] text-content-primary",
											isActive && "font-medium",
										)}
									>
										{chat.title}
									</span>
								</div>
								<div className="flex min-w-0 items-center gap-1.5">
									{hasLinkedDiffStatus && hasLineStats && (
										<span
											className="inline-flex shrink-0 items-center gap-0.5 text-[13px] leading-4 tabular-nums"
											title={`${filesChangedLabel}, +${additions} -${deletions}`}
										>
											<span className="text-git-added-bright">
												+{additions}
											</span>
											<span className="text-git-deleted-bright">
												&minus;{deletions}
											</span>
										</span>
									)}
									<div
										className={cn(
											"min-w-0 overflow-hidden text-[13px] leading-4",
											errorReason
												? "line-clamp-1 whitespace-normal text-content-destructive [overflow-wrap:anywhere]"
												: "truncate text-content-secondary",
										)}
										title={subtitle}
									>
										{subtitle}
									</div>
								</div>
							</div>
						</>
					)}
				</NavLink>
				<div className="relative mt-1 flex h-6 w-7 shrink-0 items-center justify-end">
					{isArchivingThisChat ? (
						<Spinner className="h-3.5 w-3.5 text-content-secondary" loading />
					) : (
						<>
							<span className="flex items-center justify-end text-xs text-content-secondary/50 tabular-nums [@media(hover:hover)]:group-hover:hidden group-has-[[data-state=open]]:hidden">
								{shortRelativeTime(chat.updated_at)}
							</span>
							<DropdownMenu>
								<DropdownMenuTrigger asChild>
									<Button
										size="icon"
										variant="subtle"
										className="absolute inset-0 flex h-6 w-7 min-w-0 justify-end rounded-none px-0 opacity-0 text-content-secondary hover:text-content-primary [@media(hover:hover)]:group-hover:opacity-100 data-[state=open]:opacity-100"
										aria-label={`Open actions for ${chat.title}`}
									>
										<EllipsisIcon className="h-3.5 w-3.5" />
									</Button>
								</DropdownMenuTrigger>
								<DropdownMenuContent align="end">
									{chat.archived ? (
										<DropdownMenuItem
											disabled={isArchiving}
											onSelect={() => onUnarchiveAgent(chat.id)}
										>
											<ArchiveRestoreIcon className="h-3.5 w-3.5" />
											Unarchive agent
										</DropdownMenuItem>
									) : (
										<>
											<DropdownMenuItem
												className="text-content-destructive focus:text-content-destructive"
												disabled={isArchiving}
												onSelect={() => onArchiveAgent(chat.id)}
											>
												<ArchiveIcon className="h-3.5 w-3.5" />
												Archive agent
											</DropdownMenuItem>
											{workspaceId && (
												<DropdownMenuItem
													className="text-content-destructive focus:text-content-destructive"
													disabled={isArchiving}
													onSelect={() =>
														onArchiveAndDeleteWorkspace(chat.id, workspaceId)
													}
												>
													<Trash2Icon className="h-3.5 w-3.5" />
													Archive & delete workspace
												</DropdownMenuItem>
											)}
										</>
									)}
								</DropdownMenuContent>
							</DropdownMenu>
						</>
					)}
				</div>
			</div>

			{hasChildren && isExpanded && (
				<div className="relative ml-4 border-l border-border-default/60 pl-2.5">
					{childIDs.map((childID) => {
						const childChat = chatById.get(childID);
						if (!childChat) return null;
						return (
							<ChatTreeNode key={childChat.id} chat={childChat} isChildNode />
						);
					})}
				</div>
			)}
		</div>
	);
});
ChatTreeNode.displayName = "ChatTreeNode";

export const AgentsSidebar: FC<AgentsSidebarProps> = (props) => {
	const {
		chats,
		chatErrorReasons,
		modelOptions,
		modelConfigs,
		logoUrl,
		onArchiveAgent,
		onUnarchiveAgent,
		onArchiveAndDeleteWorkspace,
		onBeforeNewAgent,
		isCreating,
		isArchiving = false,
		archivingChatId = null,
		isLoading = false,
		loadError,
		onRetryLoad,
		hasNextPage,
		onLoadMore,
		isFetchingNextPage,
		archivedFilter,
		onArchivedFilterChange,
		onCollapse,
		isAdmin = false,
	} = props;
	const { agentId, chatId } = useParams<{
		agentId?: string;
		chatId?: string;
	}>();
	const activeChatId = agentId ?? chatId;
	const { user, signOut } = useAuthenticated();
	const { appearance, buildInfo } = useDashboard();
	const location = useLocation();
	const sidebarView = sidebarViewFromPath(location.pathname);
	const normalizedSearch = "";
	const [expandedById, setExpandedById] = useState<Record<string, boolean>>({});

	const chatTree = useMemo(() => buildChatTree(chats), [chats]);
	const chatById = useMemo(() => {
		return new Map(chats.map((chat) => [chat.id, chat] as const));
	}, [chats]);
	const visibleChatIDs = useMemo(
		() =>
			collectVisibleChatIDs({
				chats,
				search: normalizedSearch,
				tree: chatTree,
			}),
		[chats, chatTree],
	);
	const visibleRootIDs = useMemo(
		() => chatTree.rootIds.filter((chatID) => visibleChatIDs.has(chatID)),
		[chatTree.rootIds, visibleChatIDs],
	);

	// Auto-expand ancestors of the active chat so it's always visible.
	useEffect(() => {
		if (!activeChatId) {
			return;
		}
		const toExpand: string[] = [];
		let cursor = chatTree.parentById.get(activeChatId);
		const seen = new Set<string>();
		while (cursor && !seen.has(cursor)) {
			seen.add(cursor);
			toExpand.push(cursor);
			cursor = chatTree.parentById.get(cursor);
		}
		if (toExpand.length > 0) {
			setExpandedById((prev) => {
				const next = { ...prev };
				for (const id of toExpand) {
					next[id] = true;
				}
				return next;
			});
		}
	}, [activeChatId, chatTree.parentById]);

	const toggleExpanded = useCallback((chatID: string) => {
		setExpandedById((prev) => ({ ...prev, [chatID]: !prev[chatID] }));
	}, []);

	const chatTreeCtx = useMemo<ChatTreeContextValue>(
		() => ({
			chatTree,
			chatById,
			visibleChatIDs,
			normalizedSearch,
			expandedById,
			modelOptions,
			modelConfigs,
			chatErrorReasons,
			isArchiving,
			archivingChatId,
			toggleExpanded,
			onArchiveAgent,
			onUnarchiveAgent,
			onArchiveAndDeleteWorkspace,
		}),
		[
			chatTree,
			chatById,
			visibleChatIDs,
			expandedById,
			modelOptions,
			modelConfigs,
			chatErrorReasons,
			isArchiving,
			archivingChatId,
			toggleExpanded,
			onArchiveAgent,
			onUnarchiveAgent,
			onArchiveAndDeleteWorkspace,
		],
	);

	const subNavTitle = "Settings";

	return (
		<div className="relative flex h-full w-full min-h-0 border-0 border-r border-solid overflow-hidden">
			{/* ── Panel 1: Chats ── */}
			<div
				className={cn(
					"absolute inset-0 flex flex-col transition-transform duration-200 ease-in-out",
					sidebarView.panel === "settings" && "-translate-x-full",
				)}
				aria-hidden={sidebarView.panel === "settings"}
				inert={sidebarView.panel === "settings" ? true : undefined}
			>
				<div className="hidden border-b border-border-default px-3 pb-3 pt-1.5 md:block md:px-3.5">
					<div className="mb-2.5 flex items-center justify-between">
						<NavLink to="/workspaces" className="inline-flex">
							{logoUrl ? (
								<ExternalImage className="h-6" src={logoUrl} alt="Logo" />
							) : (
								<CoderIcon className="h-6 w-6 fill-content-primary" />
							)}
						</NavLink>
						<div className="flex items-center gap-0.5 -mr-1.5">
							<Button
								asChild
								variant="subtle"
								size="icon"
								aria-label="Settings"
								className={cn(
									"h-7 w-7 min-w-0 text-content-secondary hover:text-content-primary",
									sidebarView.panel === "settings" && "text-content-primary",
								)}
							>
								<Link to="/agents/settings" state={{ from: location.pathname }}>
									<SettingsIcon />
								</Link>
							</Button>
							<Button
								asChild
								variant="subtle"
								size="icon"
								aria-label="Analytics"
								className={cn(
									"h-7 w-7 min-w-0 text-content-secondary hover:text-content-primary",
									sidebarView.panel === "analytics" && "text-content-primary",
								)}
							>
								<Link to="/agents/analytics">
									<BarChart3Icon />
								</Link>
							</Button>{" "}
							<DropdownMenu>
								<DropdownMenuTrigger asChild>
									<Button
										variant="subtle"
										size="icon"
										aria-label="Filter agents"
										className={cn(
											"h-7 w-7 min-w-0 text-content-secondary hover:text-content-primary",
											archivedFilter === "archived" && "text-content-primary",
										)}
									>
										<FilterIcon />
									</Button>
								</DropdownMenuTrigger>
								<DropdownMenuContent align="end">
									<DropdownMenuItem
										onSelect={() => onArchivedFilterChange?.("active")}
									>
										Active
										{archivedFilter === "active" && (
											<CheckIcon className="ml-auto h-3.5 w-3.5" />
										)}
									</DropdownMenuItem>
									<DropdownMenuItem
										onSelect={() => onArchivedFilterChange?.("archived")}
									>
										Archived
										{archivedFilter === "archived" && (
											<CheckIcon className="ml-auto h-3.5 w-3.5" />
										)}
									</DropdownMenuItem>
								</DropdownMenuContent>
							</DropdownMenu>
							{onCollapse && (
								<Button
									variant="subtle"
									size="icon"
									onClick={onCollapse}
									aria-label="Collapse sidebar"
									className="h-7 w-7 min-w-0 text-content-secondary hover:text-content-primary"
								>
									<PanelLeftCloseIcon />
								</Button>
							)}
						</div>
					</div>
					<SettingsNavItem
						icon={SquarePenIcon}
						label="New Agent"
						active={!activeChatId && sidebarView.panel === "chats"}
						to="/agents"
						onClick={onBeforeNewAgent}
						disabled={isCreating}
					/>
				</div>
				<ScrollArea
					className="flex-1 [&_[data-radix-scroll-area-viewport]>div]:!block"
					scrollBarClassName="w-1.5"
				>
					<div className="flex flex-col gap-2 px-2 py-3 md:px-2">
						{loadError ? (
							<div className="space-y-3 px-1">
								<ErrorAlert error={loadError} />
								{onRetryLoad && (
									<Button size="sm" variant="outline" onClick={onRetryLoad}>
										Retry
									</Button>
								)}
							</div>
						) : isLoading ? (
							<>
								<Skeleton className="ml-2.5 h-3.5 w-16" />
								<div className="flex flex-col gap-0.5">
									{Array.from({ length: 6 }, (_, i) => (
										<div
											key={i}
											className="flex items-start gap-2 rounded-md px-2 py-1"
										>
											<Skeleton className="mt-0.5 h-5 w-5 shrink-0 rounded-md" />
											<div className="min-w-0 flex-1 space-y-1.5">
												<Skeleton
													className="h-3.5"
													style={{ width: `${55 + ((i * 17) % 35)}%` }}
												/>
												<Skeleton className="h-3 w-20" />
											</div>
										</div>
									))}
								</div>
							</>
						) : (
							<ChatTreeContext.Provider value={chatTreeCtx}>
								{visibleRootIDs.length === 0 ? (
									<div className="rounded-lg border border-dashed border-border-default bg-surface-primary p-4 text-center text-xs text-content-secondary">
										{normalizedSearch
											? "No matching agents"
											: archivedFilter === "archived"
												? "No archived agents"
												: "No agents yet"}
									</div>
								) : (
									<div>
										{visibleRootIDs.length > 0 && (
											<div className="pb-2">
												{TIME_GROUPS.map((group) => {
													const groupChats = visibleRootIDs
														.map((id) => chatById.get(id))
														.filter(
															(chat): chat is Chat =>
																chat !== undefined &&
																getTimeGroup(chat.updated_at) === group,
														);
													if (groupChats.length === 0) return null;
													return (
														<div
															key={group}
															className="[&:not(:first-child)]:mt-3"
														>
															<div className="mb-1 ml-2.5 flex items-center justify-between text-xs font-medium text-content-secondary">
																<span>{group}</span>
															</div>
															<div className="flex flex-col gap-0.5">
																{groupChats.map((chat) => (
																	<ChatTreeNode
																		key={chat.id}
																		chat={chat}
																		isChildNode={false}
																	/>
																))}
															</div>
														</div>
													);
												})}
											</div>
										)}
									</div>
								)}
								{(hasNextPage || isFetchingNextPage) && (
									<LoadMoreSentinel
										onLoadMore={onLoadMore}
										isFetchingNextPage={isFetchingNextPage}
									/>
								)}
							</ChatTreeContext.Provider>
						)}
					</div>
				</ScrollArea>
				<div className="hidden border-0 border-t border-solid md:block">
					<div className="flex items-center">
						<DropdownMenu>
							<DropdownMenuTrigger asChild>
								<button
									type="button"
									className="flex min-w-0 flex-1 items-center gap-2 bg-transparent border-0 cursor-pointer px-3 py-3 text-left hover:bg-surface-tertiary/50 transition-colors"
								>
									<Avatar
										fallback={user.username}
										src={user.avatar_url}
										size="sm"
										className="rounded-full"
									/>
									<span className="truncate text-sm text-content-secondary">
										{user.name || user.username}
									</span>
								</button>
							</DropdownMenuTrigger>
							<DropdownMenuContent
								align="start"
								className="min-w-auto w-[260px]"
							>
								<UserDropdownContent
									user={user}
									buildInfo={buildInfo}
									supportLinks={
										appearance.support_links?.filter(
											(link) => link.location !== "navbar",
										) ?? []
									}
									onSignOut={signOut}
								/>
							</DropdownMenuContent>
						</DropdownMenu>
					</div>
				</div>
			</div>

			{/* ── Panel 2: Sub-navigation (Settings) ── */}
			<div
				className={cn(
					"absolute inset-0 flex flex-col transition-transform duration-200 ease-in-out",
					sidebarView.panel !== "settings" && "translate-x-full",
				)}
				aria-hidden={sidebarView.panel !== "settings"}
				inert={sidebarView.panel !== "settings" ? true : undefined}
			>
				{/* Back header */}
				<div className="hidden border-b border-border-default px-3 py-2.5 md:block">
					<Link
						to={(location.state as { from?: string })?.from || "/agents"}
						className="flex items-center gap-1.5 rounded-md bg-transparent px-0 py-1 text-sm font-medium text-content-secondary no-underline cursor-pointer hover:text-content-primary transition-colors"
						aria-label={`Back to chats from ${subNavTitle}`}
					>
						<ChevronLeftIcon className="h-4 w-4 shrink-0" />
						{subNavTitle}
					</Link>
				</div>
				{/* Sub-navigation items */}
				{sidebarView.panel === "settings" && (
					<nav className="flex flex-col gap-0.5 px-2 py-2">
						<SettingsNavItem
							icon={UserIcon}
							label="Behavior"
							active={sidebarView.section === "behavior"}
							to="/agents/settings/behavior"
							replace
							state={location.state}
						/>
						{isAdmin && (
							<>
								<SettingsNavItem
									icon={KeyRoundIcon}
									label="Providers"
									active={sidebarView.section === "providers"}
									to="/agents/settings/providers"
									replace
									state={location.state}
									adminOnly
								/>
								<SettingsNavItem
									icon={BoxesIcon}
									label="Models"
									active={sidebarView.section === "models"}
									to="/agents/settings/models"
									replace
									state={location.state}
									adminOnly
								/>
								<SettingsNavItem
									icon={ShieldAlertIcon}
									label="Limits"
									active={sidebarView.section === "limits"}
									to="/agents/settings/limits"
									replace
									state={location.state}
									adminOnly
								/>
								<SettingsNavItem
									icon={BarChart3Icon}
									label="Usage"
									active={sidebarView.section === "usage"}
									to="/agents/settings/usage"
									replace
									state={location.state}
									adminOnly
								/>
							</>
						)}
					</nav>
				)}
			</div>
		</div>
	);
};

type SettingsNavItemProps = {
	icon: FC<{ className?: string }>;
	label: string;
	active: boolean;
	adminOnly?: boolean;
	disabled?: boolean;
} & (
	| { to: string; replace?: boolean; state?: unknown; onClick?: () => void }
	| { to?: never; replace?: never; state?: never; onClick: () => void }
);

const navItemClassName = (active: boolean, disabled: boolean | undefined) =>
	cn(
		"flex w-full items-center gap-2.5 rounded-md border-0 px-2.5 py-2 text-left text-sm cursor-pointer transition-colors no-underline",
		active
			? "bg-surface-quaternary/25 text-content-primary font-medium"
			: "bg-transparent text-content-secondary hover:bg-surface-tertiary/50 hover:text-content-primary",
		disabled && "opacity-50 pointer-events-none",
	);

const NavItemContent: FC<{
	icon: FC<{ className?: string }>;
	label: string;
	adminOnly?: boolean;
}> = ({ icon: Icon, label, adminOnly }) => (
	<>
		<Icon className="h-4 w-4 shrink-0" />
		<span className="flex flex-1 items-center gap-2">
			{label}
			{adminOnly && (
				<Tooltip>
					<TooltipTrigger asChild>
						<span className="ml-auto inline-flex">
							<ShieldIcon className="h-3 w-3 shrink-0 opacity-50" />
						</span>
					</TooltipTrigger>
					<TooltipContent side="right">Admin only</TooltipContent>
				</Tooltip>
			)}
		</span>
	</>
);

const SettingsNavItem: FC<SettingsNavItemProps> = ({
	icon,
	label,
	active,
	adminOnly,
	disabled,
	...rest
}) => {
	if (rest.to != null) {
		return (
			<Link
				to={rest.to}
				replace={rest.replace}
				state={rest.state}
				onClick={rest.onClick}
				className={navItemClassName(active, disabled)}
				aria-current={active ? "page" : undefined}
				tabIndex={disabled ? -1 : undefined}
			>
				<NavItemContent icon={icon} label={label} adminOnly={adminOnly} />
			</Link>
		);
	}

	return (
		<button
			type="button"
			onClick={rest.onClick}
			disabled={disabled}
			className={navItemClassName(active, disabled)}
			aria-current={active ? "page" : undefined}
		>
			<NavItemContent icon={icon} label={label} adminOnly={adminOnly} />
		</button>
	);
};

const LoadMoreSentinel: FC<{
	onLoadMore?: () => void;
	isFetchingNextPage?: boolean;
}> = ({ onLoadMore, isFetchingNextPage }) => {
	const sentinelRef = useRef<HTMLDivElement>(null);
	const onLoadMoreRef = useRef(onLoadMore);
	const isFetchingNextPageRef = useRef(isFetchingNextPage);

	// Keep refs in sync with the latest prop values so the
	// observer callback always reads current state without
	// needing to tear down and re-create the observer.
	useEffect(() => {
		onLoadMoreRef.current = onLoadMore;
	}, [onLoadMore]);

	useEffect(() => {
		isFetchingNextPageRef.current = isFetchingNextPage;
	}, [isFetchingNextPage]);

	useEffect(() => {
		const el = sentinelRef.current;
		if (!el) return;

		const observer = new IntersectionObserver(
			(entries) => {
				if (
					entries[0]?.isIntersecting &&
					!isFetchingNextPageRef.current &&
					onLoadMoreRef.current
				) {
					onLoadMoreRef.current();
				}
			},
			{ threshold: 0 },
		);
		observer.observe(el);
		return () => observer.disconnect();
	}, []);

	return (
		<div ref={sentinelRef} className="flex items-center justify-center py-2">
			{isFetchingNextPage && (
				<Spinner className="h-4 w-4 text-content-secondary" loading />
			)}
		</div>
	);
};
