import {
	closestCenter,
	DndContext,
	type DragEndEvent,
	type Modifier,
	MouseSensor,
	TouchSensor,
	useSensor,
	useSensors,
} from "@dnd-kit/core";
import {
	arrayMove,
	horizontalListSortingStrategy,
	SortableContext,
	useSortable,
} from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { cn } from "cn";
import {
	ArrowLeftIcon,
	ChevronDownIcon,
	MaximizeIcon,
	MinimizeIcon,
	PanelLeftIcon,
	XIcon,
} from "lucide-react";
import {
	type FC,
	type ReactNode,
	useEffect,
	useId,
	useRef,
	useState,
} from "react";
import { useOutletContext } from "react-router";
import { Button } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuRadioGroup,
	DropdownMenuRadioItem,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import type { AgentsPageOutletContext } from "../../../AgentsPageLayout";

/** A single tab definition for the sidebar panel. */
export type SidebarTab = {
	id: string;
	/** Label shown in the tab button. */
	label: string;
	/** Short text rendered as a count-style badge after the label. */
	badge?: string;
	/** The content to render when this tab is active. */
	content: ReactNode;
	onClose?: () => void;
};

type SidebarTabViewProps = {
	/** The tabs to display. */
	tabs: SidebarTab[];
	/** Whether the panel is in expanded/fullscreen mode. */
	isExpanded: boolean;
	/** Callback to toggle expanded state. */
	onToggleExpanded: () => void;
	/** Shown next to the tabs when expanded. */
	chatTitle?: string;
	/** Callback to close the panel (used on mobile). */
	onClose?: () => void;
	/**
	 * The resolved tab ID to render as active (computed by the parent
	 * with `getEffectiveTabId`). Keeping a single source of truth in the
	 * parent prevents this component's highlight from drifting from
	 * parent-side gating like `TerminalPanel.isVisible` or
	 * `DebugPanel.isVisible`.
	 */
	effectiveTabId: string | null;
	/** Called when the user switches tabs. */
	onActiveTabChange: (tabId: string) => void;
	/** Called with the full tab ID list after the user drags a tab. */
	onReorder?: (tabIds: string[]) => void;
	addTabControl?: ReactNode;
};

/** Accessible name for a tab, including its badge text when present. */
function tabName(tab: SidebarTab): string {
	return tab.badge ? `${tab.label} ${tab.badge}` : tab.label;
}

const TabBadge: FC<{ children: string; isActive: boolean }> = ({
	children,
	isActive,
}) => (
	<span
		className={cn(
			"inline-flex h-5 min-w-5 shrink-0 items-center justify-center rounded-sm px-1.5 text-xs tabular-nums",
			isActive
				? "bg-surface-tertiary text-content-primary"
				: "bg-surface-tertiary/70 text-content-secondary",
		)}
	>
		{children}
	</span>
);

type SortableTabProps = {
	tab: SidebarTab;
	tabElementId: string;
	isActive: boolean;
	/** Whether the tab may shrink below its natural width when the strip is
	 * tight. The leading tabs never shrink so their labels stay readable. */
	canShrink: boolean;
	onSelect: () => void;
};

/**
 * A bordered browser-style tab. The active tab is one pixel taller than
 * the rest so its background covers the content frame's top border and
 * the two read as one surface. The close button is only revealed on hover
 * or keyboard focus, and the label fades under it instead of overlapping.
 */
const SortableTab: FC<SortableTabProps> = ({
	tab,
	tabElementId,
	isActive,
	canShrink,
	onSelect,
}) => {
	const ref = useRef<HTMLDivElement>(null);
	const { setNodeRef, listeners, transform, transition, isDragging } =
		useSortable({ id: tab.id });
	const onClose = tab.onClose;
	const name = tabName(tab);

	useEffect(() => {
		if (isActive) {
			ref.current?.scrollIntoView({ block: "nearest", inline: "nearest" });
		}
	}, [isActive]);

	return (
		<div
			ref={(node) => {
				ref.current = node;
				setNodeRef(node);
			}}
			style={{
				transform: CSS.Translate.toString(transform),
				transition,
			}}
			className={cn(
				"group relative flex items-center rounded-t-lg border border-b-0 border-solid border-border-default",
				canShrink ? "min-w-16 max-w-52 shrink" : "max-w-64 shrink-0",
				isActive
					? "z-10 h-[calc(2.25rem+1px)] bg-surface-primary text-content-primary"
					: "h-9 bg-surface-secondary/60 text-content-secondary hover:bg-surface-secondary hover:text-content-primary",
				isDragging && "z-20 opacity-80 shadow-md",
			)}
		>
			<button
				type="button"
				id={tabElementId}
				role="tab"
				aria-selected={isActive}
				aria-label={tab.badge ? name : undefined}
				onClick={onSelect}
				{...listeners}
				className="flex h-full min-w-0 flex-1 cursor-pointer touch-none items-center rounded-t-lg border-0 bg-transparent p-0 pl-4 pr-1 text-left text-sm text-inherit outline-hidden focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-content-link"
			>
				<span
					className={cn(
						"flex min-w-0 flex-1 items-center",
						onClose &&
							"group-hover:[mask-image:linear-gradient(to_right,black_calc(100%-2.5rem),transparent_calc(100%-1.25rem))] group-has-focus-visible:[mask-image:linear-gradient(to_right,black_calc(100%-2.5rem),transparent_calc(100%-1.25rem))]",
					)}
				>
					<span className="min-w-0 overflow-hidden whitespace-nowrap pr-3 [mask-image:linear-gradient(to_right,black_calc(100%-0.75rem),transparent)]">
						{tab.label}
					</span>
					{tab.badge && (
						<span className="-ml-1.5 mr-3 flex shrink-0">
							<TabBadge isActive={isActive}>{tab.badge}</TabBadge>
						</span>
					)}
				</span>
			</button>
			{onClose && (
				<button
					type="button"
					onClick={onClose}
					aria-label={`Close ${name} tab`}
					className="absolute top-1/2 right-2 flex size-5 -translate-y-1/2 cursor-pointer items-center justify-center rounded-sm border-0 bg-transparent p-0 text-content-secondary opacity-0 transition-opacity hover:bg-surface-quaternary hover:text-content-primary focus-visible:opacity-100 group-hover:opacity-100 group-has-focus-visible:opacity-100"
				>
					<XIcon className="size-3.5" />
				</button>
			)}
		</div>
	);
};

type TabListMenuProps = {
	tabs: SidebarTab[];
	effectiveTabId: string | null;
	onActiveTabChange: (tabId: string) => void;
};

const TabListMenu: FC<TabListMenuProps> = ({
	tabs,
	effectiveTabId,
	onActiveTabChange,
}) => {
	const [open, setOpen] = useState(false);
	return (
		<DropdownMenu open={open} onOpenChange={setOpen}>
			<DropdownMenuTrigger asChild>
				<Button
					variant="outline"
					size="icon"
					aria-label="All tabs"
					className="size-8 shrink-0 text-content-secondary hover:text-content-primary"
				>
					<ChevronDownIcon
						className={cn("size-4 transition-transform", open && "rotate-180")}
					/>
				</Button>
			</DropdownMenuTrigger>
			<DropdownMenuContent
				align="start"
				side="bottom"
				className="max-h-80 w-56 overflow-y-auto p-1 [&_[role^=menuitem]]:py-1.5 [&_[role^=menuitem]]:text-xs"
			>
				<DropdownMenuRadioGroup
					value={effectiveTabId ?? undefined}
					onValueChange={onActiveTabChange}
				>
					{tabs.map((tab) => (
						<DropdownMenuRadioItem
							key={tab.id}
							value={tab.id}
							aria-label={tab.badge ? tabName(tab) : undefined}
							className="gap-2"
						>
							<span className="truncate">{tab.label}</span>
							{tab.badge && (
								<TabBadge isActive={effectiveTabId === tab.id}>
									{tab.badge}
								</TabBadge>
							)}
						</DropdownMenuRadioItem>
					))}
				</DropdownMenuRadioGroup>
			</DropdownMenuContent>
		</DropdownMenu>
	);
};

const restrictToHorizontalAxis: Modifier[] = [
	({ transform }) => ({ ...transform, y: 0 }),
];

/** Tabs in the leading positions keep their full label; later tabs shrink
 * and fade first when the strip runs out of room. */
const FULL_WIDTH_TAB_COUNT = 3;

export const SidebarTabView: FC<SidebarTabViewProps> = ({
	tabs,
	isExpanded,
	onToggleExpanded,
	chatTitle,
	onClose,
	effectiveTabId,
	onActiveTabChange,
	onReorder,
	addTabControl,
}) => {
	const { isSidebarCollapsed, onToggleSidebarCollapsed } =
		useOutletContext<AgentsPageOutletContext | undefined>() ?? {};
	const tabIdPrefix = useId();
	const showChatTitle = isExpanded && Boolean(chatTitle);
	const tabIds = tabs.map((tab) => tab.id);

	// No keyboard sensor: dnd-kit binds Space and Enter to start a drag,
	// which would swallow the keys that activate a tab.
	const sensors = useSensors(
		useSensor(MouseSensor, { activationConstraint: { distance: 5 } }),
		useSensor(TouchSensor, {
			activationConstraint: { delay: 200, tolerance: 5 },
		}),
	);

	const handleDragEnd = ({ active, over }: DragEndEvent) => {
		if (!over || active.id === over.id) {
			return;
		}
		const oldIndex = tabIds.indexOf(String(active.id));
		const newIndex = tabIds.indexOf(String(over.id));
		if (oldIndex === -1 || newIndex === -1) {
			return;
		}
		onReorder?.(arrayMove(tabIds, oldIndex, newIndex));
	};

	return (
		<div className="flex h-full min-w-0 flex-col overflow-hidden bg-surface-primary">
			{/* The strip overlaps the frame's top border by one pixel so the
			    active tab can paint over it. */}
			<div
				role="tablist"
				className="relative z-10 -mb-px flex shrink-0 items-start gap-1.5 px-3 pt-2"
			>
				{onClose && (
					<Button
						variant="subtle"
						size="icon"
						onClick={onClose}
						aria-label="Close panel"
						className="mt-0.5 size-8 shrink-0 lg:hidden"
					>
						<ArrowLeftIcon />
					</Button>
				)}
				<DndContext
					sensors={sensors}
					collisionDetection={closestCenter}
					modifiers={restrictToHorizontalAxis}
					onDragEnd={handleDragEnd}
				>
					<SortableContext
						items={tabIds}
						strategy={horizontalListSortingStrategy}
					>
						<div
							className={cn(
								"flex min-h-[calc(2.25rem+1px)] min-w-0 items-start gap-1 overflow-x-auto scrollbar-none [&::-webkit-scrollbar]:hidden",
								showChatTitle && "max-w-[55%]",
							)}
						>
							{tabs.map((tab, index) => (
								<SortableTab
									key={tab.id}
									tab={tab}
									tabElementId={`${tabIdPrefix}-tab-${tab.id}`}
									isActive={effectiveTabId === tab.id}
									canShrink={index >= FULL_WIDTH_TAB_COUNT}
									onSelect={() => onActiveTabChange(tab.id)}
								/>
							))}
						</div>
					</SortableContext>
				</DndContext>
				<div className="mt-0.5 flex shrink-0 items-center gap-1.5">
					{tabs.length > 0 && (
						<TabListMenu
							tabs={tabs}
							effectiveTabId={effectiveTabId}
							onActiveTabChange={onActiveTabChange}
						/>
					)}
					{addTabControl}
				</div>
				{showChatTitle ? (
					<span className="mt-0.5 min-w-0 flex-1 truncate px-2 leading-8 text-center text-sm text-content-primary">
						{chatTitle}
					</span>
				) : (
					<div className="flex-1" />
				)}
				<div className="mt-0.5 flex shrink-0 items-center gap-0.5">
					{isExpanded && isSidebarCollapsed && onToggleSidebarCollapsed && (
						<Button
							variant="subtle"
							size="icon"
							onClick={onToggleSidebarCollapsed}
							aria-label="Expand sidebar"
							className="size-8 shrink-0"
						>
							<PanelLeftIcon />
						</Button>
					)}
					<Button
						variant="subtle"
						size="icon"
						onClick={onToggleExpanded}
						aria-label={isExpanded ? "Collapse panel" : "Expand panel"}
						className="hidden size-8 shrink-0 lg:inline-flex"
					>
						{isExpanded ? <MinimizeIcon /> : <MaximizeIcon />}
					</Button>
				</div>
			</div>
			<div className="relative mx-3 mb-3 flex min-h-0 flex-1 flex-col overflow-hidden rounded-lg border border-solid border-border-default">
				{tabs.length === 0 ? (
					<div className="flex flex-1 items-center justify-center p-6 text-center text-xs text-content-secondary">
						No panels available.
					</div>
				) : (
					tabs.map((tab) => {
						const isActive = effectiveTabId === tab.id;
						return (
							<div
								key={tab.id}
								role="tabpanel"
								aria-labelledby={`${tabIdPrefix}-tab-${tab.id}`}
								className={cn(
									"min-h-0 flex-1",
									// Keep inactive panels in the tree but invisible: a canvas xterm
									// preserves painted pixels while hidden, so switching back is instant.
									!isActive && "invisible absolute inset-0",
								)}
								inert={!isActive}
							>
								{tab.content}
							</div>
						);
					})
				)}
			</div>
		</div>
	);
};
