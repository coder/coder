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
import { PanelLeftCloseIcon, SettingsIcon, SquarePenIcon } from "lucide-react";
import { type FC, useEffect, useRef, useState } from "react";
import { Link, type Location, NavLink } from "react-router";
import type { Chat, ChatModelConfig } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { FeatureStageBadge } from "#/components/FeatureStageBadge/FeatureStageBadge";
import { ProductLogo } from "#/components/Icons/ProductLogo";
import { ScrollArea } from "#/components/ScrollArea/ScrollArea";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { cn } from "#/utils/cn";
import { getTimeGroup, TIME_GROUPS } from "../../../utils/timeGroups";
import type { ModelSelectorOption } from "../../ChatElements";
import { FilterDropdown } from "../filters/FilterDropdown";
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
	readonly onOpenRenameDialog?: (chat: Chat) => void;
	readonly isCreating: boolean;
	readonly isArchiving: boolean;
	readonly archivingChatId: string | null;
	readonly regeneratingTitleChatIds: readonly string[];
	readonly isLoading: boolean;
	readonly loadError?: unknown;
	readonly onRetryLoad?: () => void;
	readonly hasNextPage?: boolean;
	readonly onLoadMore?: () => void;
	readonly isFetchingNextPage?: boolean;
	readonly archivedFilter: "active" | "archived";
	readonly onArchivedFilterChange?: (filter: "active" | "archived") => void;
	readonly onCollapse?: () => void;
	readonly activeChatId: string | undefined;
	readonly isSettingsPanel: boolean;
	readonly isChatsActive: boolean;
	readonly location: Location;
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
	onOpenRenameDialog,
	isCreating,
	isArchiving,
	archivingChatId,
	regeneratingTitleChatIds,
	isLoading,
	loadError,
	onRetryLoad,
	hasNextPage,
	onLoadMore,
	isFetchingNextPage,
	archivedFilter,
	onArchivedFilterChange,
	onCollapse,
	activeChatId,
	isSettingsPanel,
	isChatsActive,
	location,
}) => {
	const normalizedSearch = "";
	const [expandedById, setExpandedById] = useState<Record<string, boolean>>({});
	const [collapsedSections, setCollapsedSections] = useState<
		Record<string, boolean>
	>({});

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
			<div className="hidden border-b border-border-default px-2 pb-3 pt-1.5 sm:block">
				<div className="mb-2.5 flex items-center justify-between">
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
					active={isChatsActive}
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
													archivedFilter === "archived" ? "active" : "archived",
												)
											}
										>
											{archivedFilter === "archived"
												? "← Back to active"
												: "View archived →"}
										</button>
									</div>
								) : (
									<div className="pb-2">
										<div className="mb-2 flex h-5 justify-end pr-1.5">
											<FilterDropdown
												archivedFilter={archivedFilter}
												onArchivedFilterChange={onArchivedFilterChange}
											/>
										</div>
										{pinnedChats.length > 0 && (
											<div className="[&:not(:first-child)]:mt-3">
												<ChatSectionHeader
													label={PINNED_SECTION_KEY}
													count={pinnedChats.length}
													expanded={!collapsedSections[PINNED_SECTION_KEY]}
													onToggle={() => toggleSection(PINNED_SECTION_KEY)}
													testId={getSectionToggleTestId(PINNED_SECTION_KEY)}
												/>
												{!collapsedSections[PINNED_SECTION_KEY] && (
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
											const isGroupExpanded = !collapsedSections[group];
											return (
												<div key={group} className="[&:not(:first-child)]:mt-3">
													<ChatSectionHeader
														label={group}
														count={groupChats.length}
														expanded={isGroupExpanded}
														onToggle={() => toggleSection(group)}
														testId={getSectionToggleTestId(group)}
													/>
													{isGroupExpanded && (
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
