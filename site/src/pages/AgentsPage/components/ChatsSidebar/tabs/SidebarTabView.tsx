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
	addTabControl?: ReactNode;
};

type BrowserTabProps = {
	tab: SidebarTab;
	tabElementId: string;
	isActive: boolean;
	onSelect: () => void;
};

/**
 * A browser-style tab: it shrinks with the strip, fades its label out on
 * the right instead of truncating, and only reveals its close button on
 * hover or keyboard focus. The active tab shares the content background
 * and covers the strip's bottom rule so it reads as attached to the panel.
 */
const BrowserTab: FC<BrowserTabProps> = ({
	tab,
	tabElementId,
	isActive,
	onSelect,
}) => {
	const ref = useRef<HTMLDivElement>(null);
	const onClose = tab.onClose;

	useEffect(() => {
		if (isActive) {
			ref.current?.scrollIntoView({ block: "nearest", inline: "nearest" });
		}
	}, [isActive]);

	return (
		<div
			ref={ref}
			className={cn(
				"group relative flex h-8 min-w-16 flex-[0_1_10rem] items-center rounded-t-md",
				isActive
					? "bg-surface-primary text-content-primary"
					: "text-content-secondary hover:bg-surface-tertiary/60 hover:text-content-primary",
			)}
		>
			<button
				type="button"
				id={tabElementId}
				role="tab"
				aria-selected={isActive}
				onClick={onSelect}
				className="flex h-full min-w-0 flex-1 cursor-pointer items-center rounded-t-md border-0 bg-transparent p-0 pl-3 text-left text-xs font-medium text-inherit outline-hidden focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-content-link"
			>
				<span
					className={cn(
						"min-w-0 flex-1 overflow-hidden whitespace-nowrap",
						onClose
							? "pr-7 [mask-image:linear-gradient(to_right,black_calc(100%-2.75rem),transparent_calc(100%-1.5rem))]"
							: "pr-3 [mask-image:linear-gradient(to_right,black_calc(100%-1.75rem),transparent_calc(100%-0.5rem))]",
					)}
				>
					{tab.label}
				</span>
			</button>
			{onClose && (
				<button
					type="button"
					onClick={onClose}
					aria-label={`Close ${tab.label} tab`}
					className="absolute top-1/2 right-1.5 flex size-5 -translate-y-1/2 cursor-pointer items-center justify-center rounded-sm border-0 bg-transparent p-0 text-content-secondary opacity-0 transition-opacity hover:bg-surface-quaternary hover:text-content-primary focus-visible:opacity-100 group-hover:opacity-100 group-focus-within:opacity-100"
				>
					<XIcon className="size-3" />
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
					variant="subtle"
					size="icon"
					aria-label="All tabs"
					className="size-7 shrink-0 text-content-secondary hover:text-content-primary"
				>
					<ChevronDownIcon
						className={cn(
							"size-3.5 transition-transform",
							open && "rotate-180",
						)}
					/>
				</Button>
			</DropdownMenuTrigger>
			<DropdownMenuContent
				align="start"
				side="bottom"
				className="max-h-80 w-56 overflow-y-auto p-1 [&_[role^=menuitem]]:py-1 [&_[role^=menuitem]]:text-xs"
			>
				<DropdownMenuRadioGroup
					value={effectiveTabId ?? undefined}
					onValueChange={onActiveTabChange}
				>
					{tabs.map((tab) => (
						<DropdownMenuRadioItem key={tab.id} value={tab.id}>
							<span className="truncate">{tab.label}</span>
						</DropdownMenuRadioItem>
					))}
				</DropdownMenuRadioGroup>
			</DropdownMenuContent>
		</DropdownMenu>
	);
};

export const SidebarTabView: FC<SidebarTabViewProps> = ({
	tabs,
	isExpanded,
	onToggleExpanded,
	chatTitle,
	onClose,
	effectiveTabId,
	onActiveTabChange,
	addTabControl,
}) => {
	const { isSidebarCollapsed, onToggleSidebarCollapsed } =
		useOutletContext<AgentsPageOutletContext | undefined>() ?? {};
	const tabIdPrefix = useId();
	const showChatTitle = isExpanded && Boolean(chatTitle);

	return (
		<div className="flex h-full min-w-0 flex-col overflow-hidden bg-surface-primary">
			{/* The inset shadow draws the strip's bottom rule beneath the tabs so
			    the active tab's background covers it. */}
			<div
				role="tablist"
				className="flex h-9 shrink-0 items-center gap-1 bg-surface-secondary px-1.5 shadow-[inset_0_-1px_0_0_hsl(var(--border-default))]"
			>
				{onClose && (
					<Button
						variant="subtle"
						size="icon"
						onClick={onClose}
						aria-label="Close panel"
						className="size-7 shrink-0 lg:hidden"
					>
						<ArrowLeftIcon />
					</Button>
				)}
				{isExpanded && isSidebarCollapsed && onToggleSidebarCollapsed && (
					<Button
						variant="subtle"
						size="icon"
						onClick={onToggleSidebarCollapsed}
						aria-label="Expand sidebar"
						className="size-7 shrink-0"
					>
						<PanelLeftIcon />
					</Button>
				)}
				{tabs.length > 0 && (
					<TabListMenu
						tabs={tabs}
						effectiveTabId={effectiveTabId}
						onActiveTabChange={onActiveTabChange}
					/>
				)}
				<div
					className={cn(
						"flex h-full min-w-0 items-end gap-px overflow-x-auto pt-1 scrollbar-none [&::-webkit-scrollbar]:hidden",
						showChatTitle && "max-w-[55%]",
					)}
				>
					{tabs.map((tab) => (
						<BrowserTab
							key={tab.id}
							tab={tab}
							tabElementId={`${tabIdPrefix}-tab-${tab.id}`}
							isActive={effectiveTabId === tab.id}
							onSelect={() => onActiveTabChange(tab.id)}
						/>
					))}
				</div>
				{addTabControl}
				{showChatTitle ? (
					<span className="min-w-0 flex-1 truncate px-2 text-center text-sm text-content-primary">
						{chatTitle}
					</span>
				) : (
					<div className="flex-1" />
				)}
				<Button
					variant="subtle"
					size="icon"
					onClick={onToggleExpanded}
					aria-label={isExpanded ? "Collapse panel" : "Expand panel"}
					className="hidden size-7 shrink-0 text-content-secondary hover:text-content-primary lg:inline-flex"
				>
					{isExpanded ? <MinimizeIcon /> : <MaximizeIcon />}
				</Button>
			</div>
			{tabs.length === 0 ? (
				<div className="flex flex-1 items-center justify-center p-6 text-center text-xs text-content-secondary">
					No panels available.
				</div>
			) : (
				<div className="relative flex min-h-0 flex-1 flex-col">
					{tabs.map((tab) => {
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
					})}
				</div>
			)}
		</div>
	);
};
