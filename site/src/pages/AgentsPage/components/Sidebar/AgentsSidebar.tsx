import {
	closestCenter,
	DndContext,
	type DragEndEvent,
	KeyboardSensor,
	MouseSensor,
	TouchSensor,
	useSensor,
	useSensors,
} from "@dnd-kit/core";
import {
	arrayMove,
	SortableContext,
	sortableKeyboardCoordinates,
	useSortable,
	verticalListSortingStrategy,
} from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import {
	AlertTriangleIcon,
	ArchiveIcon,
	ArchiveRestoreIcon,
	ArrowLeftIcon,
	BotIcon,
	BoxesIcon,
	CheckIcon,
	ChevronDownIcon,
	ChevronRightIcon,
	ChevronUpIcon,
	CoinsIcon,
	EllipsisVerticalIcon,
	FilterIcon,
	FlaskConicalIcon,
	KeyIcon,
	LayoutTemplateIcon,
	Loader2Icon,
	PanelLeftCloseIcon,
	PauseIcon,
	PinIcon,
	PinOffIcon,
	PlugIcon,
	ReceiptTextIcon,
	RefreshCwIcon,
	SearchIcon,
	ServerIcon,
	Settings2Icon,
	SettingsIcon,
	ShieldIcon,
	ShrinkIcon,
	SquarePenIcon,
	Trash2Icon,
	UserIcon,
} from "lucide-react";
import {
	createContext,
	type FC,
	useContext,
	useEffect,
	useEffectEvent,
	useRef,
	useState,
} from "react";
import { useQuery } from "react-query";
import { Link, NavLink, useLocation, useParams } from "react-router";
import { userChatProviderConfigs } from "#/api/queries/chats";
import type {
	Chat,
	ChatDiffStatus,
	ChatModelConfig,
	ChatStatus,
} from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Avatar } from "#/components/Avatar/Avatar";
import { Button } from "#/components/Button/Button";
import {
	ContextMenu,
	ContextMenuContent,
	ContextMenuItem,
	ContextMenuSeparator,
	ContextMenuTrigger,
} from "#/components/ContextMenu/ContextMenu";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { FeatureStageBadge } from "#/components/FeatureStageBadge/FeatureStageBadge";
import { ProductLogo } from "#/components/Icons/ProductLogo";
import { ScrollArea } from "#/components/ScrollArea/ScrollArea";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { UserDropdownContent } from "#/modules/dashboard/Navbar/UserDropdown/UserDropdownContent";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { cn } from "#/utils/cn";
import { shortRelativeTime } from "#/utils/time";
import { getNormalizedModelRef } from "../../utils/modelOptions";
import { getTimeGroup, TIME_GROUPS } from "../../utils/timeGroups";
import type { ModelSelectorOption } from "../ChatElements";
import { asString } from "../ChatElements/runtimeTypeUtils";
import { UsageIndicator } from "../UsageIndicator";
import { RenameChatDialog } from "./RenameChatDialog";
import { useSidebarWidth } from "./useSidebarWidth";

type SidebarView =
	| { panel: "chats" }
	| { panel: "settings"; section: string | undefined }
	| { panel: "settings-admin"; section: string | undefined }
	| { panel: "analytics" };

const ADMIN_SETTINGS_SECTIONS = new Set([
	"agents",
	"templates",
	"providers",
	"models",
	"mcp-servers",
	"spend",
	"insights",
	"instructions",
	"experiments",
	"lifecycle",
]);

/**
 * Derive the current sidebar view from the URL pathname.
 */
export function sidebarViewFromPath(pathname: string): SidebarView {
	if (pathname.startsWith("/agents/analytics")) {
		return { panel: "analytics" };
	}
	const settingsMatch = pathname.match(/^\/agents\/settings(?:\/([^/]+))?/);
	if (settingsMatch) {
		const section = settingsMatch[1];
		if (section === "admin") {
			return { panel: "settings-admin", section: undefined };
		}
		return {
			panel: ADMIN_SETTINGS_SECTIONS.has(section ?? "")
				? "settings-admin"
				: "settings",
			section,
		};
	}
	return { panel: "chats" };
}

export function isSettingsView(
	view: SidebarView,
): view is Extract<SidebarView, { panel: "settings" | "settings-admin" }> {
	return view.panel === "settings" || view.panel === "settings-admin";
}

interface AgentsSidebarProps {
	chats: readonly Chat[];
	chatErrorReasons: Record<string, string>;
	modelOptions: readonly ModelSelectorOption[];
	modelConfigs: readonly ChatModelConfig[];
	onArchiveAgent: (chatId: string) => void;
	onUnarchiveAgent: (chatId: string) => void;
	onArchiveAndDeleteWorkspace: (chatId: string, workspaceId: string) => void;
	onPinAgent: (chatId: string) => void;
	onUnpinAgent: (chatId: string) => void;
	onReorderPinnedAgent?: (chatId: string, pinOrder: number) => void;
	onRenameTitle?: (chatId: string, title: string) => Promise<void>;
	onProposeTitle?: (chatId: string) => Promise<string>;
	onBeforeNewAgent?: () => void;
	isCreating: boolean;
	isArchiving?: boolean;
	archivingChatId?: string | null;
	regeneratingTitleChatIds: readonly string[];
	isLoading?: boolean;
	loadError?: unknown;
	onRetryLoad?: () => void;
	hasNextPage?: boolean;
	onLoadMore?: () => void;
	isFetchingNextPage?: boolean;
	archivedFilter: "active" | "archived";
	onArchivedFilterChange?: (filter: "active" | "archived") => void;
	onCollapse?: () => void;
	isPersonalModelOverridesEnabled?: boolean;
	isAdmin?: boolean;
}

const statusConfig = {
	waiting: { icon: CheckIcon, className: "text-content-secondary" },
	pending: { icon: Loader2Icon, className: "text-content-link animate-spin" },
	running: { icon: Loader2Icon, className: "text-content-link animate-spin" },
	paused: { icon: PauseIcon, className: "text-content-warning" },
	requires_action: { icon: PauseIcon, className: "text-content-warning" },
	error: { icon: AlertTriangleIcon, className: "text-content-destructive" },
	completed: { icon: CheckIcon, className: "text-content-secondary" },
} as const;

type ChatTree = {
	readonly rootIds: readonly string[];
	readonly chatById: ReadonlyMap<string, Chat>;
	readonly childrenById: ReadonlyMap<string, readonly string[]>;
	readonly parentById: ReadonlyMap<string, string | undefined>;
};

const getStatusConfig = (status: ChatStatus) => {
	return statusConfig[status] ?? statusConfig.completed;
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
	const normalizedModelConfigID = asString(lastModelConfigID).trim();
	if (!normalizedModelConfigID) {
		return "Default model";
	}

	const modelOption = modelOptions.find(
		(option) => option.id === normalizedModelConfigID,
	);
	if (modelOption?.displayName) {
		return modelOption.displayName;
	}

	const modelConfig = modelConfigs.find(
		(config) => config.id === normalizedModelConfigID,
	);
	if (!modelConfig) {
		const legacyModelOption = modelOptions.find(
			(option) =>
				`${option.provider}:${option.model}` === normalizedModelConfigID,
		);
		if (legacyModelOption?.displayName) {
			return legacyModelOption.displayName;
		}
		return "Default model";
	}

	const displayName = asString(modelConfig.display_name).trim();
	if (displayName) {
		return displayName;
	}

	const { provider, model } = getNormalizedModelRef(modelConfig);
	if (!provider || !model) {
		return "Default model";
	}

	const fallbackModelOption = modelOptions.find(
		(option) => option.provider === provider && option.model === model,
	);
	if (fallbackModelOption?.displayName) {
		return fallbackModelOption.displayName;
	}

	return model;
};

const getChatDiffStatus = (chat: Chat): ChatDiffStatus | undefined => {
	return chat.diff_status;
};

/**
 * Synthesizes a short action summary from available chat metadata.
 * Priority: PR state description > file change summary > status text.
 */
const getActionSummary = (chat: Chat): string | undefined => {
	const ds = getChatDiffStatus(chat);
	if (ds) {
		const state = ds.pull_request_state;
		const fileCount = ds.changed_files;
		const fileWord = fileCount === 1 ? "file" : "files";
		if (state === "merged") {
			return `PR merged. ${fileCount} ${fileWord} changed`;
		}
		if (state === "closed") {
			return "PR closed";
		}
		if (ds.approved) {
			return "PR approved, ready to merge";
		}
		if (ds.changes_requested) {
			return "Changes requested on PR";
		}
		if (state === "open" && ds.pull_request_draft) {
			return `Draft PR opened. ${fileCount} ${fileWord} changed`;
		}
		if (state === "open") {
			return `PR opened. ${fileCount} ${fileWord} changed`;
		}
		// Has diff status but no PR yet (branch pushed).
		if (fileCount > 0) {
			return `${fileCount} ${fileWord} changed`;
		}
	}
	if (chat.status === "running" || chat.status === "pending") {
		return "Working...";
	}
	if (chat.status === "completed") {
		return "Finished";
	}
	if (chat.status === "paused" || chat.status === "waiting") {
		return "Paused";
	}
	if (chat.status === "requires_action") {
		return "Needs input";
	}
	return undefined;
};

const getParentChatID = (chat: Chat): string | undefined => {
	return asNonEmptyString(chat.parent_chat_id);
};

const buildChatTree = (chats: readonly Chat[]): ChatTree => {
	const chatById = new Map<string, Chat>();
	const parentById = new Map<string, string | undefined>();
	const childrenById = new Map<string, string[]>();

	// The paginated list now contains only root chats. Children
	// are embedded in each root's `children` field.
	for (const chat of chats) {
		chatById.set(chat.id, chat);
		childrenById.set(chat.id, []);
		// Guard against stale cache entries: if a flat child
		// entry appears in `chats` after its embedded parent has
		// already set its parent link, do not overwrite the link
		// with `undefined`. Without this, the defensive fallback
		// below re-adds the child to its parent's list, producing
		// a duplicate key in React rendering.
		if (!parentById.has(chat.id)) {
			parentById.set(chat.id, undefined);
		}

		if (chat.children) {
			for (const child of chat.children) {
				chatById.set(child.id, child);
				parentById.set(child.id, chat.id);
				childrenById.get(chat.id)?.push(child.id);
				// Children cannot have their own children (depth
				// capped at 1), but initialize the map entry for
				// uniform lookup.
				childrenById.set(child.id, []);
			}
		}
	}

	// Defensive fallback for cached data during rollout: if any
	// chat has a parent_chat_id that points to a chat in the list
	// but was not embedded, build the link. This handles stale
	// cache entries from before the backend change.
	for (const chat of chats) {
		const parentID = getParentChatID(chat);
		if (
			parentID &&
			parentID !== chat.id &&
			chatById.has(parentID) &&
			!parentById.get(chat.id)
		) {
			parentById.set(chat.id, parentID);
			childrenById.get(parentID)?.push(chat.id);
		}
	}

	const rootIds = chats
		.map((chat) => chat.id)
		.filter((chatID) => !parentById.get(chatID));

	return {
		rootIds,
		chatById,
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
		const allIDs = new Set(chats.map((chat) => chat.id));
		for (const chat of chats) {
			for (const child of chat.children ?? []) {
				allIDs.add(child.id);
			}
		}
		return allIDs;
	}

	const allChats = chats.flatMap((chat) => [chat, ...(chat.children ?? [])]);
	const matchedChatIDs = allChats
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
	readonly activeChatId: string | undefined;
	readonly isArchiving: boolean;
	readonly archivingChatId: string | null;
	readonly regeneratingTitleChatIds: readonly string[];
	readonly toggleExpanded: (chatID: string) => void;
	readonly onArchiveAgent: (chatId: string) => void;
	readonly onUnarchiveAgent: (chatId: string) => void;
	readonly onArchiveAndDeleteWorkspace: (
		chatId: string,
		workspaceId: string,
	) => void;
	readonly onPinAgent: (chatId: string) => void;
	readonly onUnpinAgent: (chatId: string) => void;
	readonly onOpenRenameDialog?: (chat: Chat) => void;
}

const ChatTreeContext = createContext<ChatTreeContextValue | null>(null);

function useChatTree(): ChatTreeContextValue {
	const ctx = useContext(ChatTreeContext);
	if (!ctx) {
		throw new Error("useChatTree must be used within ChatTreeContext.Provider");
	}
	return ctx;
}

// Stub data for fields not yet available from the backend.
// These will be replaced by real API data in a follow-up.
const getFakeMessageCount = (chat: Chat): number => {
	// Deterministic fake value derived from ID length.
	return (chat.id.charCodeAt(0) % 10) + 1;
};

const getFakeCostMicros = (chat: Chat): number => {
	// Deterministic fake dollar amount derived from ID.
	return ((chat.id.charCodeAt(1) || 50) % 100) * 1000000 + 450000;
};

const formatCostDollars = (micros: number): string => {
	const dollars = micros / 1000000;
	return `$ ${dollars.toFixed(2)}`;
};

interface ChatTreeNodeProps {
	readonly chat: Chat;
	readonly isChildNode: boolean;
}

const ChatTreeNode: FC<ChatTreeNodeProps> = ({ chat, isChildNode }) => {
	const location = useLocation();
	const {
		chatTree,
		chatById,
		visibleChatIDs,
		normalizedSearch,
		expandedById,
		modelOptions,
		modelConfigs,
		chatErrorReasons,
		activeChatId,
		isArchiving,
		archivingChatId,
		regeneratingTitleChatIds,
		toggleExpanded,
		onArchiveAgent,
		onUnarchiveAgent,
		onArchiveAndDeleteWorkspace,
		onPinAgent,
		onUnpinAgent,
		onOpenRenameDialog,
	} = useChatTree();
	const chatID = chat.id;
	const isActiveChat = activeChatId === chatID;
	const childIDs = (chatTree.childrenById.get(chatID) ?? []).filter((childID) =>
		visibleChatIDs.has(childID),
	);
	const hasChildren = childIDs.length > 0;
	const modelName = getModelDisplayName(
		chat.last_model_config_id,
		modelConfigs,
		modelOptions,
	);
	const errorReason =
		chat.status === "error"
			? chatErrorReasons[chat.id] || chat.last_error?.message || undefined
			: undefined;
	const { layout } = useSidebarWidth();
	const diffStatus = getChatDiffStatus(chat);
	const subtitle = errorReason || getActionSummary(chat) || modelName;
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
	const isRegeneratingThisChat = regeneratingTitleChatIds.includes(chat.id);
	const isExpanded = normalizedSearch ? true : (expandedById[chatID] ?? false);
	const hasRunningChildren = childIDs.some((id) => {
		const child = chatById.get(id);
		return child?.status === "running" || child?.status === "pending";
	});
	const isMainRunning = chat.status === "running" || chat.status === "pending";

	const renderMenuItems = ({
		Item,
		Separator,
	}: {
		Item: typeof DropdownMenuItem | typeof ContextMenuItem;
		Separator: typeof DropdownMenuSeparator | typeof ContextMenuSeparator;
	}) => (
		<>
			{!chat.archived && !isChildNode && (
				<Item
					onSelect={() =>
						chat.pin_order > 0 ? onUnpinAgent(chat.id) : onPinAgent(chat.id)
					}
				>
					{chat.pin_order > 0 ? (
						<>
							<PinOffIcon className="h-3.5 w-3.5" />
							Unpin agent
						</>
					) : (
						<>
							<PinIcon className="h-3.5 w-3.5" />
							Pin agent
						</>
					)}
				</Item>
			)}
			{chat.archived ? (
				<Item disabled={isArchiving} onSelect={() => onUnarchiveAgent(chat.id)}>
					<ArchiveRestoreIcon className="h-3.5 w-3.5" />
					Unarchive agent
				</Item>
			) : (
				<>
					{onOpenRenameDialog && (
						<Item onSelect={() => onOpenRenameDialog(chat)}>
							<SquarePenIcon className="h-3.5 w-3.5" />
							Rename chat
						</Item>
					)}
					{layout === "narrow" && hasChildren && (
						<Item onSelect={() => toggleExpanded(chatID)}>
							<BoxesIcon className="h-3.5 w-3.5" />
							Show subagents
						</Item>
					)}
					<Separator />
					<Item
						className="text-content-destructive focus:text-content-destructive"
						disabled={isArchiving}
						onSelect={() => onArchiveAgent(chat.id)}
					>
						<ArchiveIcon className="h-3.5 w-3.5" />
						Archive agent
					</Item>
					{workspaceId && (
						<Item
							className="text-content-destructive focus:text-content-destructive"
							disabled={isArchiving}
							onSelect={() => onArchiveAndDeleteWorkspace(chat.id, workspaceId)}
						>
							<Trash2Icon className="h-3.5 w-3.5" />
							Archive & delete workspace
						</Item>
					)}
				</>
			)}
		</>
	);

	return (
		<div className="flex min-w-0 flex-col">
			<ContextMenu>
				<ContextMenuTrigger asChild>
					{layout === "narrow" ? (
						<div
							data-testid={`agents-tree-node-${chat.id}`}
							className={cn(
								"group relative flex min-w-0 select-none [@media(pointer:coarse)]:[-webkit-touch-callout:none] items-start gap-1.5 rounded-md px-1.5 py-1 text-content-secondary",
								"transition-none [@media(hover:hover)]:hover:bg-surface-tertiary/50 [@media(hover:hover)]:hover:text-content-primary has-[[data-state=open]]:bg-surface-tertiary",
								"has-[[aria-current=page]]:bg-surface-quaternary/25 has-[[aria-current=page]]:text-content-primary [@media(hover:hover)]:has-[[aria-current=page]]:hover:bg-surface-quaternary/50",
								isChildNode &&
									"before:absolute before:-left-2.5 before:top-[17px] before:h-px before:w-2.5 before:bg-border-default/70",
							)}
						>
							<NavLink
								to={{
									pathname: `/agents/${chat.id}`,
									search: location.search,
								}}
								className="flex min-h-0 min-w-0 flex-1 items-start gap-2 rounded-[inherit] py-1 pr-0.5 text-inherit no-underline"
							>
								{({ isActive }) => (
									<>
										<div className="min-w-0 flex-1 overflow-hidden text-left">
											<div className="flex min-w-0 items-center gap-1.5 overflow-hidden">
												<span
													aria-busy={isRegeneratingThisChat}
													className={cn(
														"block flex-1 truncate text-sm text-content-primary",
														isActive && "font-medium",
														isRegeneratingThisChat && "animate-pulse",
													)}
												>
													{chat.title}
												</span>
												{chat.has_unread && !isActiveChat && (
													<span className="sr-only">(unread)</span>
												)}
												{isRegeneratingThisChat && (
													<span className="sr-only" role="status">
														Regenerating title…
													</span>
												)}
											</div>
											<div className="flex min-w-0 items-center gap-1.5">
												{hasLinkedDiffStatus && hasLineStats && (
													<span
														className="inline-flex shrink-0 items-center gap-0.5 text-sm leading-4 tabular-nums"
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
														"min-w-0 overflow-hidden text-sm leading-5",
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
							<div className="relative flex shrink-0 items-center justify-end py-1">
								{isArchivingThisChat ? (
									<Spinner
										className="h-3.5 w-3.5 text-content-secondary"
										loading
									/>
								) : isActiveChat ? (
									/* Active chat: show spinner if running, otherwise kebab */
									<>
										{(isMainRunning || hasRunningChildren) && (
											<div className="flex h-4 w-4 items-center justify-center">
												{isMainRunning && hasRunningChildren ? (
													<div className="relative h-4 w-4">
														<Loader2Icon
															className="absolute inset-0 h-4 w-4 animate-spin text-content-link/50"
															style={{ animationDuration: "2s" }}
														/>
														<Loader2Icon
															className="absolute inset-0 m-0.5 h-3 w-3 animate-spin text-content-primary"
															style={{ animationDirection: "reverse" }}
														/>
													</div>
												) : isMainRunning ? (
													<Loader2Icon className="h-4 w-4 animate-spin text-content-link" />
												) : (
													<div className="relative h-4 w-4">
														<Loader2Icon className="absolute inset-0 h-4 w-4 text-content-secondary/40" />
														<Loader2Icon className="absolute inset-0 m-0.5 h-3 w-3 animate-spin text-content-link" />
													</div>
												)}
											</div>
										)}
										<DropdownMenu>
											<DropdownMenuTrigger asChild>
												<Button
													size="icon"
													variant="subtle"
													className="h-6 w-7 min-w-0 shrink-0 justify-end rounded-none px-0 text-content-secondary hover:text-content-primary"
													aria-label={`Open actions for ${chat.title}`}
												>
													<EllipsisVerticalIcon className="h-3.5 w-3.5" />
												</Button>
											</DropdownMenuTrigger>
											<DropdownMenuContent
												align="end"
												className="[&_[role=menuitem]]:text-[13px]"
											>
												{renderMenuItems({
													Item: DropdownMenuItem,
													Separator: DropdownMenuSeparator,
												})}
											</DropdownMenuContent>
										</DropdownMenu>
									</>
								) : (
									<>
										{/* Status indicator with hover-overlay kebab */}
										<span className="flex items-center justify-end [@media(hover:hover)]:group-hover:hidden group-has-[[data-state=open]]:hidden">
											{isMainRunning || hasRunningChildren ? (
												<div className="flex h-4 w-4 items-center justify-center">
													{isMainRunning && hasRunningChildren ? (
														<div className="relative h-4 w-4">
															<Loader2Icon
																className="absolute inset-0 h-4 w-4 animate-spin text-content-link/50"
																style={{ animationDuration: "2s" }}
															/>
															<Loader2Icon
																className="absolute inset-0 m-0.5 h-3 w-3 animate-spin text-content-primary"
																style={{ animationDirection: "reverse" }}
															/>
														</div>
													) : isMainRunning ? (
														<Loader2Icon className="h-4 w-4 animate-spin text-content-link" />
													) : (
														<div className="relative h-4 w-4">
															<Loader2Icon className="absolute inset-0 h-4 w-4 text-content-secondary/40" />
															<Loader2Icon className="absolute inset-0 m-0.5 h-3 w-3 animate-spin text-content-link" />
														</div>
													)}
												</div>
											) : chat.has_unread ? (
												<span
													className="h-2.5 w-2.5 shrink-0 rounded-full bg-content-link"
													data-testid={`unread-indicator-${chat.id}`}
													aria-hidden="true"
												/>
											) : chat.status === "error" ? (
												<AlertTriangleIcon className="h-3.5 w-3.5 text-content-destructive" />
											) : (
												<span className="text-sm text-content-secondary/50 tabular-nums">
													{shortRelativeTime(chat.updated_at)}
												</span>
											)}
										</span>
										<DropdownMenu>
											<DropdownMenuTrigger asChild>
												<Button
													size="icon"
													variant="subtle"
													className="absolute inset-0 flex h-6 w-7 min-w-0 justify-end rounded-none px-0 text-content-secondary hover:text-content-primary opacity-0 [@media(hover:hover)]:group-hover:opacity-100 data-[state=open]:opacity-100"
													aria-label={`Open actions for ${chat.title}`}
												>
													<EllipsisVerticalIcon className="h-3.5 w-3.5" />
												</Button>
											</DropdownMenuTrigger>
											<DropdownMenuContent
												align="end"
												className="[&_[role=menuitem]]:text-[13px]"
											>
												{renderMenuItems({
													Item: DropdownMenuItem,
													Separator: DropdownMenuSeparator,
												})}
											</DropdownMenuContent>
										</DropdownMenu>
									</>
								)}
							</div>
						</div>
					) : (
						<div
							data-testid={`agents-tree-node-${chat.id}`}
							className={cn(
								"group relative flex min-w-0 select-none items-center gap-2 rounded-md px-1.5 py-1 text-content-secondary",
								"transition-none [@media(hover:hover)]:hover:bg-surface-tertiary/50 [@media(hover:hover)]:hover:text-content-primary",
								"has-[[aria-current=page]]:bg-surface-quaternary/25 has-[[aria-current=page]]:text-content-primary",
							)}
						>
							{hasChildren ? (
								<Button
									variant="subtle"
									size="icon"
									onClick={() => toggleExpanded(chatID)}
									className="h-5 w-5 shrink-0 p-0 text-content-secondary/60 hover:text-content-primary [&>svg]:size-3.5"
									aria-label={isExpanded ? "Collapse" : "Expand"}
									aria-expanded={isExpanded}
								>
									{isExpanded ? <ChevronDownIcon /> : <ChevronRightIcon />}
								</Button>
							) : (
								<div className="w-5 shrink-0" />
							)}
							<NavLink
								to={{ pathname: `/agents/${chat.id}`, search: location.search }}
								className={cn(
									"min-w-0 flex-1 truncate py-1 text-sm text-content-primary no-underline",
									isActiveChat && "font-medium",
								)}
							>
								{chat.title}
							</NavLink>
							{chat.has_unread && !isActiveChat && (
								<span
									className="h-2 w-2 shrink-0 rounded-full bg-content-link"
									data-testid={`unread-indicator-${chat.id}`}
									aria-hidden="true"
								/>
							)}
							<span
								className={cn(
									"shrink-0 truncate text-sm",
									layout === "large" ? "w-[140px]" : "w-[100px]",
									errorReason
										? "text-content-destructive"
										: "text-content-secondary",
								)}
								title={subtitle}
							>
								{subtitle}
							</span>
							{layout === "large" && (
								<span className="w-[50px] shrink-0 text-right text-xs text-content-secondary tabular-nums">
									{getFakeMessageCount(chat)} 💬
								</span>
							)}
							{hasLinkedDiffStatus && hasLineStats ? (
								<span
									className="inline-flex shrink-0 items-center gap-0.5 text-sm tabular-nums"
									title={`${filesChangedLabel}, +${additions} -${deletions}`}
								>
									<span className="text-git-added-bright">+{additions}</span>
									<span className="text-git-deleted-bright">
										&minus;{deletions}
									</span>
								</span>
							) : (
								<div className="w-[70px] shrink-0" />
							)}
							{layout === "large" && (
								<span className="w-[60px] shrink-0 text-right text-xs text-content-secondary tabular-nums">
									{formatCostDollars(getFakeCostMicros(chat))}
								</span>
							)}
							{/* Single status indicator, priority: spinner > unread > error > time */}
							<div className="flex h-5 w-5 shrink-0 items-center justify-center">
								{isArchivingThisChat ? (
									<Spinner
										className="h-3.5 w-3.5 text-content-secondary"
										loading
									/>
								) : isMainRunning && hasRunningChildren ? (
									<div className="relative h-4 w-4">
										<Loader2Icon
											className="absolute inset-0 h-4 w-4 animate-spin text-content-link/50"
											style={{ animationDuration: "2s" }}
										/>
										<Loader2Icon
											className="absolute inset-0 m-0.5 h-3 w-3 animate-spin text-content-primary"
											style={{ animationDirection: "reverse" }}
										/>
									</div>
								) : isMainRunning ? (
									<Loader2Icon className="h-4 w-4 animate-spin text-content-link" />
								) : !isMainRunning && hasRunningChildren ? (
									<div className="relative h-4 w-4">
										<Loader2Icon className="absolute inset-0 h-4 w-4 text-content-secondary/40" />
										<Loader2Icon className="absolute inset-0 m-0.5 h-3 w-3 animate-spin text-content-link" />
									</div>
								) : chat.has_unread && !isActiveChat ? (
									<span
										className="h-2.5 w-2.5 rounded-full bg-content-link"
										data-testid={`unread-indicator-wide-${chat.id}`}
									/>
								) : chat.status === "error" ? (
									<AlertTriangleIcon className="h-3.5 w-3.5 text-content-destructive" />
								) : (
									<span className="text-[11px] text-content-secondary/50 tabular-nums">
										{shortRelativeTime(chat.updated_at)}
									</span>
								)}
							</div>
							<DropdownMenu>
								<DropdownMenuTrigger asChild>
									<Button
										size="icon"
										variant="subtle"
										className="h-6 w-6 shrink-0 p-0 text-content-secondary hover:text-content-primary opacity-0 [@media(hover:hover)]:group-hover:opacity-100 data-[state=open]:opacity-100"
										aria-label={`Open actions for ${chat.title}`}
									>
										<EllipsisVerticalIcon className="h-3.5 w-3.5" />
									</Button>
								</DropdownMenuTrigger>
								<DropdownMenuContent
									align="end"
									className="[&_[role=menuitem]]:text-[13px]"
								>
									{renderMenuItems({
										Item: DropdownMenuItem,
										Separator: DropdownMenuSeparator,
									})}
								</DropdownMenuContent>
							</DropdownMenu>
						</div>
					)}
				</ContextMenuTrigger>
				<ContextMenuContent className="[&_[role=menuitem]]:text-[13px]">
					{renderMenuItems({
						Item: ContextMenuItem,
						Separator: ContextMenuSeparator,
					})}
				</ContextMenuContent>
			</ContextMenu>

			{hasChildren && isExpanded && (
				<div className="relative ml-4 border-l border-border-default/60 pl-2.5">
					{(() => {
						const children = childIDs
							.map((id) => chatById.get(id))
							.filter((c): c is Chat => c !== undefined);
						const prChildren = children.filter(
							(c) => getChatDiffStatus(c)?.url,
						);
						const subagentChildren = children.filter(
							(c) => !getChatDiffStatus(c)?.url,
						);
						return (
							<>
								{prChildren.length > 0 && (
									<div className="py-0.5">
										<div className="mb-0.5 text-xs font-medium text-content-secondary">
											{prChildren.length}{" "}
											{prChildren.length === 1 ? "PR" : "PRs"}
										</div>
										{prChildren.map((child) => {
											const ds = getChatDiffStatus(child);
											const adds = ds?.additions ?? 0;
											const dels = ds?.deletions ?? 0;
											return (
												<div
													key={child.id}
													className="flex items-center gap-2 py-0.5 text-sm"
												>
													<span className="inline-flex shrink-0 items-center gap-0.5 tabular-nums">
														<span className="text-git-added-bright">
															+{adds}
														</span>
														<span className="text-git-deleted-bright">
															&minus;{dels}
														</span>
													</span>
													<NavLink
														to={{
															pathname: `/agents/${child.id}`,
															search: location.search,
														}}
														className="min-w-0 truncate text-content-primary no-underline hover:underline"
													>
														{ds?.pull_request_title || child.title}
													</NavLink>
												</div>
											);
										})}
									</div>
								)}
								{subagentChildren.length > 0 && (
									<div className="py-0.5">
										<div className="mb-0.5 flex items-center gap-1 text-sm font-medium text-content-secondary">
											<ChevronUpIcon className="h-3.5 w-3.5 shrink-0" />
											{subagentChildren.length} Subagents
										</div>
										<div className="relative border-l border-border-default/60 ml-1">
											{subagentChildren.map((child) => {
												const childConfig = getStatusConfig(child.status);
												const ChildStatusIcon = childConfig.icon;
												const childDiff = getChatDiffStatus(child);
												const childAdds = childDiff?.additions ?? 0;
												const childDels = childDiff?.deletions ?? 0;
												const childHasStats = childAdds > 0 || childDels > 0;
												const childIsRunning =
													child.status === "running" ||
													child.status === "pending";
												return (
													<div
														key={child.id}
														className="relative flex items-center gap-2 py-0.5 pl-3 text-sm before:absolute before:left-0 before:top-1/2 before:h-px before:w-3 before:bg-border-default/60"
													>
														<NavLink
															to={{
																pathname: `/agents/${child.id}`,
																search: location.search,
															}}
															className="min-w-0 flex-1 truncate text-content-primary no-underline hover:underline"
														>
															{child.title}
														</NavLink>
														{childHasStats && (
															<ChildStatusIcon
																className={cn(
																	"h-3.5 w-3.5 shrink-0",
																	childConfig.className,
																)}
															/>
														)}
														{childIsRunning ? (
															<Loader2Icon className="h-3.5 w-3.5 shrink-0 animate-spin text-content-link" />
														) : child.has_unread ? (
															<span className="h-2.5 w-2.5 shrink-0 rounded-full bg-content-link" />
														) : null}
													</div>
												);
											})}
										</div>
									</div>
								)}
							</>
						);
					})()}
				</div>
			)}
			{hasChildren && isExpanded && !isChildNode && layout !== "narrow" && (
				<div className="ml-4 mt-1 space-y-1 border-l border-border-default/60 pl-2.5 text-xs text-content-secondary">
					<div className="flex gap-2">
						<span className="w-16 shrink-0 font-medium">Created</span>
						<span>
							{new Date(chat.created_at).toLocaleString("en-US", {
								month: "short",
								day: "numeric",
								year: "numeric",
								hour: "2-digit",
								minute: "2-digit",
								second: "2-digit",
							})}
						</span>
					</div>
					{layout !== "large" && (
						<div className="flex gap-2">
							<span className="w-16 shrink-0 font-medium">Cost</span>
							<span>{formatCostDollars(getFakeCostMicros(chat))}</span>
						</div>
					)}
					<div className="flex items-center gap-2">
						<span className="w-16 shrink-0 font-medium">Tokens</span>
						<span className="inline-flex items-center gap-1">
							<span className="rounded bg-surface-tertiary px-1.5 py-0.5 text-[11px]">
								↓ 123132
							</span>
							<span className="rounded bg-surface-tertiary px-1.5 py-0.5 text-[11px]">
								↑ 233
							</span>
						</span>
					</div>
					<div className="flex items-center gap-2">
						<span className="w-16 shrink-0 font-medium">Model</span>
						<span className="inline-flex items-center gap-1.5">
							<span className="h-2 w-2 rounded-full bg-content-link" />
							<span>{modelName}</span>
						</span>
					</div>
					{layout === "large" && (
						<div className="flex gap-2">
							<span className="w-16 shrink-0 font-medium">Initial prompt</span>
							<p className="min-w-0 flex-1 overflow-hidden rounded border border-border bg-surface-secondary p-2 text-[12px] leading-relaxed text-content-primary line-clamp-3">
								I want to create a new github workflow that is based on this
								existing one. The main purpose of my new workflow is to run when
								I assign a label...
							</p>
						</div>
					)}
				</div>
			)}
		</div>
	);
};

const SortableChatTreeNode: FC<{
	chat: Chat;
}> = ({ chat }) => {
	const {
		attributes,
		listeners,
		setNodeRef,
		transform,
		transition,
		isDragging,
	} = useSortable({
		id: chat.id,
		// Skip the derived-transform measurement after drop.
		// localPinOrder already repositions items in the DOM,
		// so the two-frame snap-back dance produces stale deltas
		// and a visible jitter. This makes items snap directly.
		animateLayoutChanges: () => false,
	});

	// Strip scaleX/scaleY that dnd-kit adds by default.
	const adjustedTransform = transform
		? { ...transform, scaleX: 1, scaleY: 1 }
		: null;

	const style = {
		transform: CSS.Transform.toString(adjustedTransform),
		transition: isDragging ? "opacity 200ms" : transition,
	};

	return (
		<div
			ref={setNodeRef}
			style={style}
			className={cn(isDragging && "opacity-50")}
			{...attributes}
			{...listeners}
		>
			<ChatTreeNode chat={chat} isChildNode={false} />
		</div>
	);
};

export const AgentsSidebar: FC<AgentsSidebarProps> = (props) => {
	const {
		chats,
		chatErrorReasons,
		modelOptions,
		modelConfigs,
		onArchiveAgent,
		onUnarchiveAgent,
		onArchiveAndDeleteWorkspace,
		onPinAgent,
		onUnpinAgent,
		onReorderPinnedAgent,
		onRenameTitle,
		onProposeTitle,
		onBeforeNewAgent,
		isCreating,
		isArchiving = false,
		archivingChatId = null,
		regeneratingTitleChatIds,
		isLoading = false,
		loadError,
		onRetryLoad,
		hasNextPage,
		onLoadMore,
		isFetchingNextPage,
		archivedFilter,
		onArchivedFilterChange,
		onCollapse,
		isPersonalModelOverridesEnabled = false,
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
	const isSettingsPanel = isSettingsView(sidebarView);
	const isFallbackToUserPanel =
		sidebarView.panel === "settings-admin" && !isAdmin;
	const settingsPanel =
		sidebarView.panel === "settings-admin" && isAdmin
			? "settings-admin"
			: "settings";
	const settingsSection =
		isSettingsPanel && !isFallbackToUserPanel ? sidebarView.section : undefined;
	const providerConfigsQuery = useQuery({
		...userChatProviderConfigs(),
		enabled: isSettingsPanel && !isAdmin,
	});
	const isApiKeysSection = isSettingsPanel && settingsSection === "api-keys";
	const showApiKeysItem =
		isAdmin || isApiKeysSection || Boolean(providerConfigsQuery.data?.length);
	const normalizedSearch = "";
	const { layout } = useSidebarWidth();
	const [expandedById, setExpandedById] = useState<Record<string, boolean>>({});
	const [chatPendingRename, setChatPendingRename] = useState<Chat | null>(null);
	const [collapsedSections, setCollapsedSections] = useState<Set<string>>(
		new Set(),
	);
	const toggleSection = (key: string) => {
		setCollapsedSections((prev) => {
			const next = new Set(prev);
			if (next.has(key)) {
				next.delete(key);
			} else {
				next.add(key);
			}
			return next;
		});
	};

	const chatTree = buildChatTree(chats);
	const chatById = chatTree.chatById;
	const visibleChatIDs = collectVisibleChatIDs({
		chats,
		search: normalizedSearch,
		tree: chatTree,
	});
	const visibleRootIDs = chatTree.rootIds.filter((chatID) =>
		visibleChatIDs.has(chatID),
	);

	const pinnedChats = visibleRootIDs
		.map((id) => chatById.get(id))
		.filter((chat): chat is Chat => (chat?.pin_order ?? 0) > 0)
		.sort((a, b) => a.pin_order - b.pin_order);

	// Local override for pinned order during drag. Applied
	// synchronously so there's no flash between the dnd-kit
	// transform clearing and the server data arriving.
	const [localPinOrder, setLocalPinOrder] = useState<string[] | null>(null);

	// Clear the local override when fresh data arrives from
	// the server (the mutation's onSettled invalidates queries).
	const chatsRef = useRef(chats);
	useEffect(() => {
		if (chats !== chatsRef.current) {
			chatsRef.current = chats;
			setLocalPinOrder(null);
		}
	}, [chats]);

	const sortedPinnedChats = localPinOrder
		? localPinOrder
				.map((id) => pinnedChats.find((c) => c.id === id))
				.filter((c) => c !== undefined)
		: pinnedChats;

	const pinnedChatIds = sortedPinnedChats.map((chat) => chat.id);

	const lastDragEndedAtRef = useRef(0);

	const pinnedContainerRef = useRef<HTMLDivElement | null>(null);
	useEffect(() => {
		const handler = (e: MouseEvent) => {
			const container = pinnedContainerRef.current;
			const target = e.target;
			if (
				container &&
				target instanceof Node &&
				container.contains(target) &&
				performance.now() - lastDragEndedAtRef.current < 300
			) {
				e.preventDefault();
			}
		};
		document.addEventListener("click", handler, true);
		return () => document.removeEventListener("click", handler, true);
	}, []);

	const sensors = useSensors(
		useSensor(MouseSensor, {
			activationConstraint: { distance: 5 },
		}),
		useSensor(TouchSensor, {
			activationConstraint: { delay: 200, tolerance: 5 },
		}),
		useSensor(KeyboardSensor, {
			coordinateGetter: sortableKeyboardCoordinates,
		}),
	);

	const handleDragEnd = (event: DragEndEvent) => {
		const { active, over } = event;

		lastDragEndedAtRef.current = performance.now();
		if (!over || active.id === over.id) return;
		const activeId = String(active.id);
		const overId = String(over.id);
		const oldIndex = pinnedChatIds.indexOf(activeId);
		const newIndex = pinnedChatIds.indexOf(overId);
		if (oldIndex === -1 || newIndex === -1) return;

		const reordered = arrayMove(pinnedChatIds, oldIndex, newIndex);
		setLocalPinOrder(reordered);
		onReorderPinnedAgent?.(activeId, newIndex + 1);
	};

	const filterDropdown = (
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
			<DropdownMenuContent
				align="end"
				className="mobile-full-width-dropdown mobile-full-width-dropdown-top-below-header [&_[role=menuitem]]:text-[13px]"
			>
				<DropdownMenuItem onSelect={() => onArchivedFilterChange?.("active")}>
					Active
					{archivedFilter === "active" && (
						<CheckIcon className="ml-auto h-3.5 w-3.5" />
					)}
				</DropdownMenuItem>
				<DropdownMenuItem onSelect={() => onArchivedFilterChange?.("archived")}>
					Archived
					{archivedFilter === "archived" && (
						<CheckIcon className="ml-auto h-3.5 w-3.5" />
					)}
				</DropdownMenuItem>
			</DropdownMenuContent>
		</DropdownMenu>
	);

	// Auto-expand ancestors of the active chat so it's always visible.
	// Only runs when activeChatId changes, not on every parentById
	// recalculation, so user-initiated collapse is preserved.
	const parentByIdRef = useRef(chatTree.parentById);
	useEffect(() => {
		parentByIdRef.current = chatTree.parentById;
	});
	useEffect(() => {
		if (!activeChatId) {
			return;
		}
		const parentById = parentByIdRef.current;
		const toExpand: string[] = [];
		let cursor = parentById.get(activeChatId);
		const seen = new Set<string>();
		while (cursor && !seen.has(cursor)) {
			seen.add(cursor);
			toExpand.push(cursor);
			cursor = parentById.get(cursor);
		}
		if (toExpand.length > 0) {
			setExpandedById((prev) => {
				if (toExpand.every((id) => prev[id])) {
					return prev;
				}
				const next = { ...prev };
				for (const id of toExpand) {
					next[id] = true;
				}
				return next;
			});
		}
	}, [activeChatId]);
	const toggleExpanded = (chatID: string) => {
		setExpandedById((prev) => ({ ...prev, [chatID]: !prev[chatID] }));
	};

	const chatTreeCtx: ChatTreeContextValue = {
		chatTree,
		chatById,
		visibleChatIDs,
		normalizedSearch,
		expandedById,
		modelOptions,
		modelConfigs,
		chatErrorReasons,
		activeChatId,
		isArchiving,
		archivingChatId,
		regeneratingTitleChatIds,
		toggleExpanded,
		onArchiveAgent,
		onUnarchiveAgent,
		onArchiveAndDeleteWorkspace,
		onPinAgent,
		onUnpinAgent,
		onOpenRenameDialog: onRenameTitle ? setChatPendingRename : undefined,
	};

	const subNavTitle =
		settingsPanel === "settings-admin" ? "Manage Agents" : "Settings";
	return (
		<div className="relative flex h-full w-full min-h-0 border-0 border-r border-solid overflow-hidden">
			{/* ── Panel 1: Chats ── */}
			<div
				className={cn(
					"absolute inset-0 flex flex-col sm:transition-transform sm:duration-200 sm:ease-in-out",
					isSettingsPanel && "-translate-x-full",
				)}
				aria-hidden={isSettingsPanel}
				inert={isSettingsPanel ? true : undefined}
			>
				<div className="hidden border-b border-border-default px-2 pb-3 pt-3 sm:block">
					<div className="mb-2.5 flex items-center justify-between px-1.5">
						<div className="flex items-center gap-2">
							<NavLink to="/workspaces" className="inline-flex">
								<ProductLogo className="size-6" />
							</NavLink>
							<FeatureStageBadge
								contentType="beta"
								size="sm"
								className="flex items-center py-0.5 leading-normal"
							/>
						</div>
						<div className="flex items-center gap-0.5 -mr-1.5">
							<Button
								asChild
								variant="subtle"
								size="icon"
								aria-label="Settings"
								className={cn(
									"h-7 w-7 min-w-0 text-content-secondary hover:text-content-primary",
									isSettingsPanel && "text-content-primary",
								)}
							>
								<Link
									to="/agents/settings"
									state={{ from: location.pathname + location.search }}
								>
									<SettingsIcon />
								</Link>
							</Button>
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
						to={`/agents${location.search}`}
						onClick={onBeforeNewAgent}
						disabled={isCreating}
					/>
				</div>
				<div className="relative min-h-0 flex-1">
					<ScrollArea
						className="h-full [&_[data-radix-scroll-area-viewport]>div]:!block"
						scrollBarClassName="w-1.5"
						viewportClassName={cn(
							"[mask-image:linear-gradient(to_bottom,transparent_0,black_20px,black_calc(100%-20px),transparent_100%)]",
							"[-webkit-mask-image:linear-gradient(to_bottom,transparent_0,black_20px,black_calc(100%-20px),transparent_100%)]",
							"sm:[mask-image:none] sm:[-webkit-mask-image:none]",
						)}
					>
						<div className="flex flex-col gap-2 px-2 py-3">
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
												className="flex items-start gap-2 rounded-md px-1.5 py-1"
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
								<ChatTreeContext value={chatTreeCtx}>
									{visibleRootIDs.length === 0 ? (
										<div className="rounded-lg border border-dashed border-border-default bg-surface-primary p-4 text-center text-xs text-content-secondary">
											<p className="m-0">
												{normalizedSearch
													? "No matching agents"
													: archivedFilter === "archived"
														? "No archived agents"
														: "No agents yet"}
											</p>
											<button
												type="button"
												className="mt-2 cursor-pointer border-none bg-transparent p-0 text-xs text-content-secondary hover:text-content-primary hover:underline"
												onClick={() =>
													onArchivedFilterChange?.(
														archivedFilter === "archived"
															? "active"
															: "archived",
													)
												}
											>
												{archivedFilter === "archived"
													? "← Back to active"
													: "View archived →"}
											</button>
										</div>
									) : (
										<div>
											{/* At narrow widths, show only the filter icon.
								   At medium/large, show full search input with filter. */}
											{layout === "narrow" ? (
												<div className="mb-2 flex justify-end">
													{filterDropdown}
												</div>
											) : (
												<div className="mb-2 flex items-center gap-1.5">
													<div className="relative flex-1">
														<SearchIcon className="absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-content-secondary" />
														<input
															type="text"
															placeholder="Search chats..."
															className="h-8 w-full rounded-md border border-border bg-transparent pl-8 pr-2 text-xs text-content-primary placeholder:text-content-secondary focus:border-content-link focus:outline-none"
															readOnly
														/>
													</div>
													{filterDropdown}
												</div>
											)}
											{visibleRootIDs.length > 0 && (
												<div className="pb-2">
													{/* ── Pinned section ── */}
													{pinnedChats.length > 0 && (
														<div className="[&:not(:first-child)]:mt-3">
															<button
																type="button"
																onClick={() => toggleSection("Pinned")}
																className="mb-1 flex w-full items-center justify-between border-0 bg-transparent px-1.5 text-sm font-normal leading-6 text-content-secondary hover:text-content-primary cursor-pointer"
															>
																<span className="flex items-center gap-1.5">
																	Pinned ({pinnedChats.length})
																	{collapsedSections.has("Pinned") &&
																		pinnedChats.some((c) => c.has_unread) && (
																			<span
																				className="h-2 w-2 shrink-0 rounded-full bg-content-link"
																				aria-hidden="true"
																			/>
																		)}
																</span>
																<span
																	className={cn(
																		"h-4 w-4 shrink-0 transition-transform duration-200",
																		collapsedSections.has("Pinned") &&
																			"rotate-180",
																	)}
																>
																	<ChevronUpIcon className="h-4 w-4" />
																</span>
															</button>
															{!collapsedSections.has("Pinned") && (
																<DndContext
																	sensors={sensors}
																	collisionDetection={closestCenter}
																	onDragEnd={handleDragEnd}
																>
																	<SortableContext
																		items={pinnedChatIds}
																		strategy={verticalListSortingStrategy}
																	>
																		<div
																			ref={pinnedContainerRef}
																			className="flex flex-col gap-0.5"
																		>
																			{sortedPinnedChats.map((chat) => (
																				<SortableChatTreeNode
																					key={chat.id}
																					chat={chat}
																				/>
																			))}
																		</div>
																	</SortableContext>
																</DndContext>
															)}
														</div>
													)}
													{/* ── Time-grouped sections ── */}
													{TIME_GROUPS.map((group) => {
														const groupChats = visibleRootIDs
															.map((id) => chatById.get(id))
															.filter(
																(chat): chat is Chat =>
																	chat !== undefined &&
																	getTimeGroup(chat.updated_at) === group &&
																	chat.pin_order === 0,
															);
														if (groupChats.length === 0) return null;
														return (
															<div
																key={group}
																className="[&:not(:first-child)]:mt-3"
															>
																<button
																	type="button"
																	onClick={() => toggleSection(group)}
																	className="mb-1 flex w-full items-center justify-between border-0 bg-transparent px-1.5 text-sm font-normal leading-6 text-content-secondary hover:text-content-primary cursor-pointer"
																>
																	<span className="flex items-center gap-1.5">
																		{group} ({groupChats.length})
																		{collapsedSections.has(group) &&
																			groupChats.some((c) => c.has_unread) && (
																				<span
																					className="h-2 w-2 shrink-0 rounded-full bg-content-link"
																					aria-hidden="true"
																				/>
																			)}
																	</span>
																	<span
																		className={cn(
																			"h-4 w-4 shrink-0 transition-transform duration-200",
																			collapsedSections.has(group) &&
																				"rotate-180",
																		)}
																	>
																		<ChevronUpIcon className="h-4 w-4" />
																	</span>
																</button>
																{!collapsedSections.has(group) && (
																	<div className="flex flex-col gap-0.5">
																		{groupChats.map((chat) => (
																			<ChatTreeNode
																				key={chat.id}
																				chat={chat}
																				isChildNode={false}
																			/>
																		))}
																	</div>
																)}
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
								</ChatTreeContext>
							)}
						</div>
					</ScrollArea>
				</div>
				<div className="hidden border-0 border-t border-solid sm:block">
					<div className="flex items-stretch">
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
						<UsageIndicator />
					</div>
				</div>
			</div>
			{/* ── Panel 2: Sub-navigation (Settings) ── */}
			<div
				className={cn(
					"absolute inset-0 flex flex-col sm:transition-transform sm:duration-200 sm:ease-in-out",
					!isSettingsPanel && "translate-x-full",
				)}
				aria-hidden={!isSettingsPanel}
				inert={!isSettingsPanel ? true : undefined}
			>
				{/* Back header */}
				<div className="border-b border-border-default px-2 pb-2 pt-3 sm:py-2">
					<div className="relative flex items-center">
						<span className="pointer-events-none absolute inset-0 flex items-center justify-center text-sm font-medium text-content-primary">
							{subNavTitle}
						</span>
						<Button
							asChild
							variant="subtle"
							size="icon"
							aria-label={
								settingsPanel === "settings-admin"
									? "Back to Settings"
									: "Back to Agents"
							}
							className="relative z-10 h-7 w-7 min-w-0 text-content-secondary hover:text-content-primary"
						>
							{settingsPanel === "settings-admin" ? (
								<Link
									to="/agents/settings/general"
									state={location.state}
									aria-label="Back to Settings"
								>
									<ArrowLeftIcon />
								</Link>
							) : (
								<Link
									to={(location.state as { from?: string })?.from || "/agents"}
								>
									<ArrowLeftIcon />
								</Link>
							)}
						</Button>
						<div className="flex-1" />
						{onCollapse && (
							<Button
								variant="subtle"
								size="icon"
								onClick={onCollapse}
								aria-label="Collapse sidebar"
								className="relative z-10 hidden h-7 w-7 min-w-0 text-content-secondary hover:text-content-primary sm:inline-flex"
							>
								<PanelLeftCloseIcon />
							</Button>
						)}
					</div>
				</div>
				{/* Sub-navigation items */}
				{settingsPanel === "settings" ? (
					<nav className="flex flex-col gap-0.5 px-2 py-2">
						<SettingsNavItem
							icon={UserIcon}
							label="General"
							active={!settingsSection || settingsSection === "general"}
							to="/agents/settings/general"
							state={location.state}
						/>
						{isPersonalModelOverridesEnabled && (
							<SettingsNavItem
								icon={BotIcon}
								label="Agents"
								active={settingsSection === "user-agents"}
								to="/agents/settings/user-agents"
								state={location.state}
							/>
						)}
						<SettingsNavItem
							icon={ShrinkIcon}
							label="Compaction"
							active={settingsSection === "compaction"}
							to="/agents/settings/compaction"
							state={location.state}
						/>
						{showApiKeysItem && (
							<SettingsNavItem
								icon={KeyIcon}
								label="Secrets (API keys)"
								active={settingsSection === "api-keys"}
								to="/agents/settings/api-keys"
								state={location.state}
							/>
						)}
						{isAdmin && (
							<SettingsNavItem
								icon={Settings2Icon}
								label="Manage Agents"
								active={false}
								to="/agents/settings/admin"
								state={location.state}
								trailingIcon={ChevronRightIcon}
							/>
						)}
					</nav>
				) : (
					<nav className="flex flex-col gap-0.5 px-2 py-2">
						<SettingsNavItem
							icon={BotIcon}
							label="Agents"
							active={!settingsSection || settingsSection === "agents"}
							to="/agents/settings/agents"
							state={location.state}
						/>
						<SettingsNavItem
							icon={PlugIcon}
							label="Providers"
							active={settingsSection === "providers"}
							to="/agents/settings/providers"
							state={location.state}
						/>
						<SettingsNavItem
							icon={BoxesIcon}
							label="Models"
							active={settingsSection === "models"}
							to="/agents/settings/models"
							state={location.state}
						/>
						<SettingsNavItem
							icon={ServerIcon}
							label="MCP Servers"
							active={settingsSection === "mcp-servers"}
							to="/agents/settings/mcp-servers"
							state={location.state}
						/>
						<SettingsNavItem
							icon={LayoutTemplateIcon}
							label="Templates"
							active={settingsSection === "templates"}
							to="/agents/settings/templates"
							state={location.state}
						/>
						<SettingsNavItem
							icon={CoinsIcon}
							label="Spend"
							active={settingsSection === "spend"}
							to="/agents/settings/spend"
							state={location.state}
						/>
						<SettingsNavItem
							icon={ReceiptTextIcon}
							label="Instructions"
							active={settingsSection === "instructions"}
							to="/agents/settings/instructions"
							state={location.state}
						/>
						<SettingsNavItem
							icon={FlaskConicalIcon}
							label="Experiments"
							active={settingsSection === "experiments"}
							to="/agents/settings/experiments"
							state={location.state}
						/>
						<SettingsNavItem
							icon={RefreshCwIcon}
							label="Lifecycle"
							active={settingsSection === "lifecycle"}
							to="/agents/settings/lifecycle"
							state={location.state}
						/>
					</nav>
				)}
			</div>
			{onRenameTitle && (
				<RenameChatDialog
					chat={chatPendingRename}
					onRename={onRenameTitle}
					onPropose={onProposeTitle}
					onOpenChange={(open) => {
						if (!open) setChatPendingRename(null);
					}}
				/>
			)}
		</div>
	);
};

type SettingsNavItemProps = {
	icon: FC<{ className?: string }>;
	label: string;
	active: boolean;
	adminOnly?: boolean;
	disabled?: boolean;
	trailingIcon?: FC<{ className?: string }>;
} & (
	| { to: string; replace?: boolean; state?: unknown; onClick?: () => void }
	| { to?: never; replace?: never; state?: never; onClick: () => void }
);

const navItemClassName = (active: boolean, disabled: boolean | undefined) =>
	cn(
		"flex w-full items-center gap-2.5 rounded-md border-0 px-1.5 py-2 text-left text-sm cursor-pointer transition-colors no-underline",
		active
			? "bg-surface-quaternary/25 text-content-primary font-medium"
			: "bg-transparent text-content-secondary hover:bg-surface-tertiary/50 hover:text-content-primary",
		disabled && "opacity-50 pointer-events-none",
	);

const NavItemContent: FC<{
	icon: FC<{ className?: string }>;
	label: string;
	adminOnly?: boolean;
	trailingIcon?: FC<{ className?: string }>;
}> = ({ icon: Icon, label, adminOnly, trailingIcon: TrailingIcon }) => (
	<>
		<Icon className="h-4 w-4 shrink-0" />
		<span className="min-w-0 flex-1">{label}</span>
		{(adminOnly || TrailingIcon) && (
			<span className="ml-auto flex items-center gap-2">
				{adminOnly && (
					<Tooltip>
						<TooltipTrigger asChild>
							<span className="inline-flex">
								<ShieldIcon className="h-3 w-3 shrink-0 opacity-50" />
							</span>
						</TooltipTrigger>
						<TooltipContent side="right">Admin only</TooltipContent>
					</Tooltip>
				)}
				{TrailingIcon && <TrailingIcon className="h-4 w-4 shrink-0" />}
			</span>
		)}
	</>
);

const SettingsNavItem: FC<SettingsNavItemProps> = ({
	icon,
	label,
	active,
	adminOnly,
	disabled,
	trailingIcon,
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
				<NavItemContent
					icon={icon}
					label={label}
					adminOnly={adminOnly}
					trailingIcon={trailingIcon}
				/>
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
			<NavItemContent
				icon={icon}
				label={label}
				adminOnly={adminOnly}
				trailingIcon={trailingIcon}
			/>
		</button>
	);
};

const LoadMoreSentinel: FC<{
	onLoadMore?: () => void;
	isFetchingNextPage?: boolean;
}> = ({ onLoadMore, isFetchingNextPage }) => {
	const sentinelRef = useRef<HTMLDivElement>(null);
	const onLoadMoreEvent = useEffectEvent(() => {
		onLoadMore?.();
	});

	useEffect(() => {
		// Don't observe while a fetch is in progress. When the
		// fetch completes this effect re-runs, creating a fresh
		// observer whose initial entry detects the sentinel if
		// it's still visible, fixing the case where loaded items
		// don't push the sentinel out of view and the previous
		// observer never re-fires.
		if (isFetchingNextPage) return;

		const el = sentinelRef.current;
		if (!el) return;

		const observer = new IntersectionObserver(
			(entries) => {
				if (entries[0]?.isIntersecting) {
					onLoadMoreEvent();
				}
			},
			{ threshold: 0 },
		);
		observer.observe(el);
		return () => observer.disconnect();
	}, [isFetchingNextPage]);

	return (
		<div ref={sentinelRef} className="flex items-center justify-center py-2">
			{isFetchingNextPage && (
				<Spinner className="h-4 w-4 text-content-secondary" loading />
			)}
		</div>
	);
};
