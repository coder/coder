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
	verticalListSortingStrategy,
} from "@dnd-kit/sortable";
import {
	PanelLeftCloseIcon,
	SearchIcon,
	SettingsIcon,
	SquarePenIcon,
} from "lucide-react";
import { type FC, useEffect, useRef, useState } from "react";
import { Link, type Location, NavLink } from "react-router";
import type { Chat, ChatModelConfig } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { FeatureStageBadge } from "#/components/FeatureStageBadge/FeatureStageBadge";
import { ProductLogo } from "#/components/Icons/ProductLogo";
import { Kbd, KbdGroup } from "#/components/Kbd/Kbd";
import { ScrollArea } from "#/components/ScrollArea/ScrollArea";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { cn } from "#/utils/cn";
import { getOSKey } from "#/utils/platform";
import {
	AGENT_CHAT_STATUS_ORDER,
	type AgentSidebarFilters,
	DEFAULT_AGENT_SIDEBAR_FILTERS,
} from "../../../utils/agentSidebarFilters";
import { getTimeGroup, TIME_GROUPS } from "../../../utils/timeGroups";
import type { ModelSelectorOption } from "../../ChatElements";
import { FilterPopover } from "../filters/FilterPopover";
import { normalizeLocationSearch } from "../locationSearch";
import { SettingsNavItem } from "../settings/SettingsNavItem";
import {
	ChatTreeContext,
	type ChatTreeContextValue,
} from "../tree/ChatTreeContext";
import { ChatTreeNode } from "../tree/ChatTreeNode";
import {
	buildChatTree,
	type ChatTree,
	collectVisibleChatIDs,
} from "../tree/chatTree";
import { SortableChatTreeNode } from "../tree/SortableChatTreeNode";
import {
	ChatSectionHeader,
	getSectionToggleTestId,
	PINNED_SECTION_KEY,
} from "./ChatSectionHeader";
import { LoadMoreSentinel } from "./LoadMoreSentinel";
import { UserSidebarFooter } from "./UserSidebarFooter";

const UNREAD_SECTION_KEY = "Unread";
const READ_SECTION_KEY = "Read";
const SHARED_WITH_YOU_SECTION_KEY = "Shared with you";

interface ChatsPanelProps {
	readonly chats: readonly Chat[];
	readonly chatErrorReasons: Record<string, string>;
	readonly modelOptions: readonly ModelSelectorOption[];
	readonly modelConfigs: readonly ChatModelConfig[];
	readonly onArchiveAgent: (chatId: string) => void;
	readonly onUnarchiveAgent: (chatId: string) => void;
	readonly onArchiveAndDeleteWorkspace: (
		chatId: string,
		workspaceId: string,
	) => void;
	readonly onPinAgent: (chatId: string) => void;
	readonly onUnpinAgent: (chatId: string) => void;
	readonly onReorderPinnedAgent?: (chatId: string, pinOrder: number) => void;
	readonly onBeforeNewAgent?: () => void;
	readonly onOpenSearchDialog?: () => void;
	readonly onOpenRenameDialog?: (chat: Chat) => void;
	readonly isCreating: boolean;
	readonly isArchiving: boolean;
	readonly archivingChatId: string | null;
	readonly isLoading: boolean;
	readonly loadError?: unknown;
	readonly onRetryLoad?: () => void;
	readonly hasNextPage?: boolean;
	readonly onLoadMore?: () => void;
	readonly isFetchingNextPage?: boolean;
	readonly sidebarFilters: AgentSidebarFilters;
	readonly onSidebarFiltersChange: (filters: AgentSidebarFilters) => void;
	readonly onCollapse?: () => void;
	readonly activeChatId: string | undefined;
	readonly isSettingsPanel: boolean;
	readonly isChatsActive: boolean;
	readonly location: Location;
	readonly currentUserId: string;
}

export const ChatsPanel: FC<ChatsPanelProps> = ({
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
	onBeforeNewAgent,
	onOpenSearchDialog,
	onOpenRenameDialog,
	isCreating,
	isArchiving,
	archivingChatId,
	isLoading,
	loadError,
	onRetryLoad,
	hasNextPage,
	onLoadMore,
	isFetchingNextPage,
	sidebarFilters,
	onSidebarFiltersChange,
	onCollapse,
	activeChatId,
	isSettingsPanel,
	isChatsActive,
	location,
	currentUserId,
}) => {
	const locationSearch = normalizeLocationSearch(location.search);
	const [expandedById, setExpandedById] = useState<Record<string, boolean>>({});
	const [collapsedSections, setCollapsedSections] = useState<
		Record<string, boolean>
	>({});

	const chatTree = buildChatTree(chats);
	const chatById = chatTree.chatById;
	const visibleChatIDs = collectVisibleChatIDs({
		chats,
		search: "",
		tree: chatTree,
	});
	const visibleRootIDs = chatTree.rootIds.filter((chatID) =>
		visibleChatIDs.has(chatID),
	);

	const pinnedChats = visibleRootIDs
		.map((id) => chatById.get(id))
		.filter((chat): chat is Chat => (chat?.pin_order ?? 0) > 0)
		.sort((a, b) => a.pin_order - b.pin_order);
	const unpinnedChats = visibleRootIDs
		.map((id) => chatById.get(id))
		.filter((chat): chat is Chat => chat !== undefined && chat.pin_order === 0);
	const sharedWithYouChats = unpinnedChats.filter(
		(chat) => chat.shared && chat.owner_id !== currentUserId,
	);
	const unpinnedOwnedChats = unpinnedChats.filter(
		(chat) => !chat.shared || chat.owner_id === currentUserId,
	);
	const hasAppliedResultFilters =
		sidebarFilters.prStatuses.length > 0 ||
		sidebarFilters.chatStatuses.length !== AGENT_CHAT_STATUS_ORDER.length ||
		sidebarFilters.sources.length !==
			DEFAULT_AGENT_SIDEBAR_FILTERS.sources.length ||
		sidebarFilters.sources.some(
			(source) => !DEFAULT_AGENT_SIDEBAR_FILTERS.sources.includes(source),
		);
	const disablePinnedReordering = hasAppliedResultFilters;

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

		if (disablePinnedReordering) {
			return;
		}

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

	// Auto-expand ancestors of the active chat so it's always visible.
	// Only runs when activeChatId changes, not on every parentById
	// recalculation, so user-initiated collapse is preserved.
	const parentByIdRef = useRef<ChatTree["parentById"]>(chatTree.parentById);
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
	const toggleSection = (sectionKey: string) => {
		setCollapsedSections((prev) => ({
			...prev,
			[sectionKey]: !prev[sectionKey],
		}));
	};

	const chatTreeCtx: ChatTreeContextValue = {
		chatTree,
		chatById,
		visibleChatIDs,
		normalizedSearch: "",
		expandedById,
		modelOptions,
		modelConfigs,
		chatErrorReasons,
		activeChatId,
		isArchiving,
		archivingChatId,
		toggleExpanded,
		onArchiveAgent,
		onUnarchiveAgent,
		onArchiveAndDeleteWorkspace,
		onPinAgent,
		onUnpinAgent,
		onOpenRenameDialog,
	};

	const chatSections = (
		sidebarFilters.groupBy === "chat_status"
			? [
					{
						key: UNREAD_SECTION_KEY,
						label: UNREAD_SECTION_KEY,
						chats: unpinnedOwnedChats.filter((chat) => chat.has_unread),
					},
					{
						key: READ_SECTION_KEY,
						label: READ_SECTION_KEY,
						chats: unpinnedOwnedChats.filter((chat) => !chat.has_unread),
					},
				]
			: TIME_GROUPS.map((group) => ({
					key: group,
					label: group,
					chats: unpinnedOwnedChats.filter(
						(chat) => getTimeGroup(chat.updated_at) === group,
					),
				}))
	).filter((section) => section.chats.length > 0);
	const isShowingEmptyState = visibleRootIDs.length === 0;
	const isViewingArchived = sidebarFilters.archiveStatus === "archived";
	const chatsHeadingLabel = isViewingArchived ? "Archived chats" : "Chats";
	const emptyStateMessage = hasAppliedResultFilters
		? "No agents match these filters"
		: isViewingArchived
			? "No archived agents"
			: "No agents yet";
	const clearResultFilters = () => {
		onSidebarFiltersChange({
			...sidebarFilters,
			prStatuses: [],
			chatStatuses: AGENT_CHAT_STATUS_ORDER,
			sources: DEFAULT_AGENT_SIDEBAR_FILTERS.sources,
		});
	};

	return (
		<div
			className={cn(
				"absolute inset-0 flex flex-col sm:transition-transform sm:duration-200 sm:ease-in-out",
				isSettingsPanel && "-translate-x-full",
			)}
			aria-hidden={isSettingsPanel}
			inert={isSettingsPanel ? true : undefined}
		>
			<nav
				aria-label="Sidebar"
				className="hidden border-b border-border-default px-2 py-1.5 sm:flex sm:flex-col sm:gap-0.5"
			>
				<div className="flex items-center justify-between mb-2.5 ml-2.5">
					<div className="flex items-center gap-2">
						<NavLink to="/workspaces" className="inline-flex">
							<ProductLogo className="size-6" />
						</NavLink>
						<FeatureStageBadge contentType="beta" size="xs" />
					</div>
					<div className="flex items-center gap-0.5 -mr-1.5">
						<Button
							asChild
							variant="subtle"
							size="icon"
							aria-label="Settings"
							className={cn(
								"size-7 min-w-0 text-content-secondary hover:text-content-primary",
								isSettingsPanel && "text-content-primary",
							)}
						>
							<Link
								to="/agents/settings"
								state={{ from: location.pathname + locationSearch }}
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
								className="size-7 min-w-0 text-content-secondary hover:text-content-primary"
							>
								<PanelLeftCloseIcon />
							</Button>
						)}
					</div>
				</div>
				<SettingsNavItem
					icon={SquarePenIcon}
					label="New chat"
					active={isChatsActive}
					to={{ pathname: "/agents", search: locationSearch }}
					onClick={onBeforeNewAgent}
					disabled={isCreating}
				/>
				{onOpenSearchDialog && (
					<SettingsNavItem
						icon={SearchIcon}
						label="Search"
						active={false}
						ariaLabel="Search chats"
						onClick={onOpenSearchDialog}
						className="group focus-visible:bg-surface-tertiary/50 focus-visible:text-content-primary"
						trailing={
							<KbdGroup className="opacity-0 transition-opacity group-hover:opacity-100 group-focus-visible:opacity-100">
								<Kbd>{getOSKey()}</Kbd>
								<Kbd>K</Kbd>
							</KbdGroup>
						}
					/>
				)}
			</nav>
			<div className="relative min-h-0 flex-1 flex flex-col">
				<div className="mx-2 pt-6 mb-1.5">
					<div className="ml-2.5 mr-2 flex h-7 items-center justify-between">
						<h2 className="m-0 text-sm font-normal leading-6 text-content-secondary">
							{chatsHeadingLabel}
						</h2>
						<div className="flex items-center gap-1">
							{onOpenSearchDialog && (
								<Button
									variant="subtle"
									size="icon"
									aria-label="Search chats"
									onClick={onOpenSearchDialog}
									className="size-7 sm:hidden"
								>
									<SearchIcon />
								</Button>
							)}
							<FilterPopover
								filters={sidebarFilters}
								onFiltersChange={onSidebarFiltersChange}
							/>
						</div>
					</div>
				</div>
				<ScrollArea
					className="min-h-0 flex-1 [&_[data-radix-scroll-area-viewport]>div]:!block"
					scrollBarClassName="w-1.5"
					// The default 24px hit-target extends ~18px left of this narrow
					// scrollbar, onto the row controls (actions menu, timestamp,
					// indicators). Disable it so those controls stay clickable.
					scrollThumbClassName="before:hidden"
					viewportClassName={cn(
						"[mask-image:linear-gradient(to_bottom,transparent_0,black_20px,black_calc(100%-20px),transparent_100%)]",
						"[-webkit-mask-image:linear-gradient(to_bottom,transparent_0,black_20px,black_calc(100%-20px),transparent_100%)]",
						"sm:[mask-image:none] sm:[-webkit-mask-image:none]",
					)}
				>
					<div className="flex flex-col gap-2 px-2 pb-3">
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
											<Skeleton className="mt-0.5 size-5 shrink-0 rounded-md" />
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
								<div className="pb-2">
									{isShowingEmptyState ? (
										<div className="rounded-lg border border-dashed border-border-default bg-surface-primary p-4 text-center text-xs text-content-secondary">
											<p className="m-0">{emptyStateMessage}</p>
											{hasAppliedResultFilters && (
												<button
													type="button"
													className="mt-2 cursor-pointer border-none bg-transparent p-0 text-xs text-content-secondary hover:text-content-primary hover:underline"
													onClick={clearResultFilters}
												>
													Clear filters
												</button>
											)}
										</div>
									) : (
										<>
											{pinnedChats.length > 0 && (
												<div className="[&:not(:first-child)]:mt-3">
													<ChatSectionHeader
														label={PINNED_SECTION_KEY}
														count={pinnedChats.length}
														expanded={!collapsedSections[PINNED_SECTION_KEY]}
														onToggle={() => toggleSection(PINNED_SECTION_KEY)}
														testId={getSectionToggleTestId(PINNED_SECTION_KEY)}
													/>
													{!collapsedSections[PINNED_SECTION_KEY] &&
														(disablePinnedReordering ? (
															<div className="flex flex-col gap-0.5">
																{sortedPinnedChats.map((chat) => (
																	<ChatTreeNode
																		key={chat.id}
																		chat={chat}
																		isChildNode={false}
																	/>
																))}
															</div>
														) : (
															<DndContext
																sensors={sensors}
																collisionDetection={closestCenter}
																modifiers={[
																	// Restrict the drag to the y-axis only
																	({ transform }) => ({
																		...transform,
																		x: 0,
																	}),
																]}
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
														))}
												</div>
											)}
											{sharedWithYouChats.length > 0 && (
												<div className="[&:not(:first-child)]:mt-3">
													<ChatSectionHeader
														label={SHARED_WITH_YOU_SECTION_KEY}
														count={sharedWithYouChats.length}
														expanded={
															!collapsedSections[SHARED_WITH_YOU_SECTION_KEY]
														}
														onToggle={() =>
															toggleSection(SHARED_WITH_YOU_SECTION_KEY)
														}
														testId={getSectionToggleTestId(
															SHARED_WITH_YOU_SECTION_KEY,
														)}
													/>
													{!collapsedSections[SHARED_WITH_YOU_SECTION_KEY] && (
														<div className="flex flex-col gap-0.5">
															{sharedWithYouChats.map((chat) => (
																<ChatTreeNode
																	key={chat.id}
																	chat={chat}
																	isChildNode={false}
																/>
															))}
														</div>
													)}
												</div>
											)}
											{chatSections.map((section) => {
												const isSectionExpanded =
													!collapsedSections[section.key];
												return (
													<div
														key={section.key}
														className="[&:not(:first-child)]:mt-3"
													>
														<ChatSectionHeader
															label={section.label}
															count={section.chats.length}
															expanded={isSectionExpanded}
															onToggle={() => toggleSection(section.key)}
															testId={getSectionToggleTestId(section.key)}
														/>
														{isSectionExpanded && (
															<div className="flex flex-col gap-0.5">
																{section.chats.map((chat) => (
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
										</>
									)}
								</div>
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
			<UserSidebarFooter />
		</div>
	);
};
