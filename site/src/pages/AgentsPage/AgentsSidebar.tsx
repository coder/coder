import type { Chat, ChatDiffStatus, ChatStatus } from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
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
import { Input } from "components/Input/Input";
import { ScrollArea } from "components/ScrollArea/ScrollArea";
import { Skeleton } from "components/Skeleton/Skeleton";
import {
	AlertTriangleIcon,
	ArchiveIcon,
	CheckIcon,
	ChevronDownIcon,
	ChevronRightIcon,
	EllipsisIcon,
	Loader2Icon,
	PauseIcon,
	SearchIcon,
} from "lucide-react";
import {
	type FC,
	type ReactNode,
	useCallback,
	useEffect,
	useMemo,
	useState,
} from "react";
import { NavLink, useParams } from "react-router";
import { cn } from "utils/cn";
import { shortRelativeTime } from "utils/time";

interface AgentsSidebarProps {
	chats: readonly Chat[];
	chatErrorReasons: Record<string, string>;
	modelOptions: readonly ModelSelectorOption[];
	logoUrl?: string;
	onArchiveAgent: (chatId: string) => void;
	onNewAgent: () => void;
	isCreating: boolean;
	isArchiving?: boolean;
	archivingChatId?: string | null;
	isLoading?: boolean;
	loadError?: unknown;
	onRetryLoad?: () => void;
}

const statusConfig = {
	waiting: { icon: CheckIcon, className: "text-content-secondary" },
	pending: { icon: Loader2Icon, className: "text-content-link animate-spin" },
	running: { icon: Loader2Icon, className: "text-content-link animate-spin" },
	paused: { icon: PauseIcon, className: "text-content-warning" },
	error: { icon: AlertTriangleIcon, className: "text-content-destructive" },
	completed: { icon: CheckIcon, className: "text-content-secondary" },
} as const;

type ChatWithExtendedMetadata = Chat & {
	readonly diff_status?: ChatDiffStatus;
	readonly parent_chat_id?: string;
	readonly root_chat_id?: string;
};

type ChatTree = {
	readonly rootIds: readonly string[];
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
	modelConfig: Chat["model_config"] | undefined,
	modelOptions: readonly ModelSelectorOption[],
) => {
	if (!modelConfig || typeof modelConfig !== "object") {
		return "Default model";
	}

	const model = (modelConfig as { model?: string }).model;
	if (!model) {
		return "Default model";
	}

	// Try to find a matching option with a display name.
	const match = modelOptions.find(
		(opt) => opt.id === model || opt.model === model,
	);
	if (match?.displayName) {
		return match.displayName;
	}

	// Fall back to stripping the provider prefix.
	const parts = model.split(":");
	if (parts.length === 2) {
		return parts[1];
	}

	return model;
};

const getChatDiffStatus = (chat: Chat): ChatDiffStatus | undefined => {
	return (chat as ChatWithExtendedMetadata).diff_status;
};

const getParentChatID = (chat: Chat): string | undefined => {
	return asNonEmptyString((chat as ChatWithExtendedMetadata).parent_chat_id);
};

const getRootChatID = (chat: Chat): string | undefined => {
	return asNonEmptyString((chat as ChatWithExtendedMetadata).root_chat_id);
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

export const AgentsSidebar: FC<AgentsSidebarProps> = (props) => {
	const {
		chats,
		chatErrorReasons,
		modelOptions,
		logoUrl,
		onArchiveAgent,
		onNewAgent,
		isCreating,
		isArchiving = false,
		archivingChatId = null,
		isLoading = false,
		loadError,
		onRetryLoad,
	} = props;
	const { chatId: activeChatId } = useParams<{ chatId: string }>();
	const [search, setSearch] = useState("");
	const normalizedSearch = search.trim().toLowerCase();
	const [expandedById, setExpandedById] = useState<Record<string, boolean>>(
		{},
	);

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
		[chats, normalizedSearch, chatTree],
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

	const renderChatNode = (chatID: string, isChildNode: boolean): ReactNode => {
		const chat = chatById.get(chatID);
		if (!chat || !visibleChatIDs.has(chatID)) {
			return null;
		}

		const childIDs = (chatTree.childrenById.get(chatID) ?? []).filter((childID) =>
			visibleChatIDs.has(childID),
		);
		const hasChildren = childIDs.length > 0;
		const isDelegated = Boolean(getParentChatID(chat));
		const config = getStatusConfig(chat.status);
		const StatusIcon = config.icon;
		const isDelegatedExecuting =
			isDelegated && (chat.status === "pending" || chat.status === "running");
		const modelName = getModelDisplayName(chat.model_config, modelOptions);
		const errorReason =
			chat.status === "error" ? chatErrorReasons[chat.id] : undefined;
		const subtitle = errorReason || modelName;
		const diffStatus = getChatDiffStatus(chat);
		const hasLinkedDiffStatus = Boolean(diffStatus?.url);
		const changedFiles = diffStatus?.changed_files ?? 0;
		const additions = diffStatus?.additions ?? 0;
		const deletions = diffStatus?.deletions ?? 0;
		const hasLineStats = additions > 0 || deletions > 0;
		const filesChangedLabel = `${changedFiles} ${
			changedFiles === 1 ? "file" : "files"
		}`;
		const isArchivingThisChat = isArchiving && archivingChatId === chat.id;

		const isExpanded = normalizedSearch
			? true
			: expandedById[chatID] ?? false;

		return (
			<div key={chat.id} className="flex min-w-0 flex-col">
				<div
					data-testid={`agents-tree-node-${chat.id}`}
					className={cn(
						"group relative flex min-w-0 items-start gap-1.5 rounded-md pr-1 text-content-secondary",
						"transition-none hover:bg-surface-tertiary/50 hover:text-content-primary has-[[data-state=open]]:bg-surface-tertiary",
						"has-[[aria-current=page]]:bg-surface-quaternary/25 has-[[aria-current=page]]:text-content-primary has-[[aria-current=page]]:hover:bg-surface-quaternary/50",
						isChildNode &&
							"before:absolute before:-left-2.5 before:top-[17px] before:h-px before:w-2.5 before:bg-border-default/70",
					)}
				>
					<div className="relative mt-1.5 h-5 w-5 shrink-0">
						<div
							className={cn(
								"flex h-5 w-5 items-center justify-center rounded-md",
								hasChildren && "group-hover:invisible",
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
								className="absolute inset-0 invisible flex h-5 w-5 min-w-0 items-center justify-center rounded-md p-0 text-content-secondary/60 hover:text-content-primary group-hover:visible [&>svg]:size-3.5"
								data-testid={`agents-tree-toggle-${chat.id}`}
								aria-label={isExpanded ? "Collapse" : "Expand"}
								aria-expanded={isExpanded}
							>
								{isExpanded ? (
									<ChevronDownIcon />
								) : (
									<ChevronRightIcon />
								)}
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
												className="inline-flex shrink-0 items-center gap-0.5 text-[13px] font-medium leading-none tabular-nums"
												title={`${filesChangedLabel}, +${additions} -${deletions}`}
											>
												<span className="text-content-success">
													+{additions}
												</span>
												<span className="text-content-destructive">
													-{deletions}
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
					<div className="relative mr-1 mt-1 h-6 w-7 shrink-0 text-right">
						<span className="absolute inset-0 flex items-center justify-end text-xs text-content-secondary/50 tabular-nums transition-opacity group-hover:opacity-0">
							{shortRelativeTime(chat.updated_at)}
						</span>
						<DropdownMenu>
							<DropdownMenuTrigger asChild>
								<Button
									size="icon"
									variant="subtle"
									className={cn(
										"absolute inset-0 h-6 w-7 justify-end rounded-none px-0 text-content-secondary opacity-0 transition-opacity hover:text-content-primary group-hover:opacity-100 group-focus-within:opacity-100 focus-visible:opacity-100 data-[state=open]:opacity-100",
										isArchivingThisChat && "opacity-100",
									)}
									aria-label={`Open actions for ${chat.title}`}
								>
									{isArchivingThisChat ? (
										<Loader2Icon className="h-3.5 w-3.5 animate-spin" />
									) : (
										<EllipsisIcon className="h-3.5 w-3.5" />
									)}
								</Button>
							</DropdownMenuTrigger>
							<DropdownMenuContent align="end">
								<DropdownMenuItem onSelect={(event) => event.preventDefault()}>
									Mark as unread
								</DropdownMenuItem>
								<DropdownMenuItem
									className="text-content-destructive focus:text-content-destructive"
									disabled={isArchiving}
									onSelect={() => onArchiveAgent(chat.id)}
								>
									<ArchiveIcon className="h-3.5 w-3.5" />
									Archive agent
								</DropdownMenuItem>
							</DropdownMenuContent>
						</DropdownMenu>
					</div>
				</div>

			{hasChildren && isExpanded && (
				<div className="relative ml-4 border-l border-border-default/60 pl-2.5">
					{childIDs.map((childID) => renderChatNode(childID, true))}
				</div>
			)}
			</div>
		);
	};

	return (
		<div className="flex h-full w-full min-h-0 flex-col border-0 border-r border-solid">
			<div className="border-b border-border-default px-3 pb-3 pt-1.5 md:px-3.5">
				<NavLink to="/workspaces" className="mb-2.5 inline-flex">
					{logoUrl ? (
						<ExternalImage className="h-6" src={logoUrl} alt="Logo" />
					) : (
						<CoderIcon className="h-6 w-6 fill-content-primary" />
					)}
				</NavLink>
				<div className="flex flex-col gap-2.5">
					<div className="relative">
						<label className="sr-only" htmlFor="agents-sidebar-search">
							Search agents...
						</label>
						<SearchIcon className="pointer-events-none absolute left-3 top-1/2 size-icon-xs -translate-y-1/2 text-content-secondary" />
						<Input
							id="agents-sidebar-search"
							type="search"
							placeholder="Search agents..."
							value={search}
							onChange={(event) => setSearch(event.target.value)}
							className="h-9 rounded-lg border-border-default bg-surface-primary pl-8 text-[13px] shadow-none"
						/>
					</div>
					<Button
						size="sm"
						variant="outline"
						onClick={onNewAgent}
						disabled={isCreating}
						className="w-full justify-center rounded-lg py-4 text-[13px] text-content-secondary hover:bg-surface-tertiary"
					>
						New Agent
					</Button>
				</div>
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
						<>
							<div className="ml-2.5 flex items-center justify-between text-xs font-medium text-content-secondary">
								<span>This Week</span>
							</div>

							<div className="flex flex-col gap-0.5">
								{visibleRootIDs.map((chatID) => renderChatNode(chatID, false))}

								{visibleRootIDs.length === 0 && (
									<div className="rounded-lg border border-dashed border-border-default bg-surface-primary p-4 text-center text-xs text-content-secondary">
										{normalizedSearch ? "No matching agents" : "No agents yet"}
									</div>
								)}
							</div>
						</>
					)}
				</div>
			</ScrollArea>
		</div>
	);
};
