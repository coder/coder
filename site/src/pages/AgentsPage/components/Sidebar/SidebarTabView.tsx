import {
	ChevronLeftIcon,
	ChevronRightIcon,
	MaximizeIcon,
	MinimizeIcon,
	PanelLeftIcon,
	XIcon,
} from "lucide-react";
import type { ReactNode } from "react";
import { type FC, useEffect, useId, useRef, useState } from "react";
import { Button } from "#/components/Button/Button";
import { cn } from "#/utils/cn";
import { DesktopPanel } from "../RightPanel/DesktopPanel";

/** A single tab definition for the sidebar panel. */
export interface SidebarTab {
	id: string;
	/** Label shown in the tab button. */
	label: string;
	/** Optional icon shown before the label. */
	icon?: ReactNode;
	/** Optional badge shown after the label (e.g. diff stats). */
	badge?: ReactNode;
	/** The content to render when this tab is active. */
	content: ReactNode;
}

interface SidebarTabViewProps {
	/** The tabs to display. */
	tabs: SidebarTab[];
	/** Whether the panel is in expanded/fullscreen mode. */
	isExpanded: boolean;
	/** Callback to toggle expanded state. */
	onToggleExpanded: () => void;
	/** Whether the left sidebar is collapsed. */
	isSidebarCollapsed?: boolean;
	/** Callback to toggle left sidebar. */
	onToggleSidebarCollapsed?: () => void;
	/** Shown in center when expanded. */
	chatTitle?: string;
	/** Callback to close the panel (used on mobile). */
	onClose?: () => void;
	/** Desktop chat ID. Omitted if desktop is not available. */
	desktopChatId?: string;
	/** The currently active tab ID (controlled by the parent). */
	activeTabId: string | null;
	/** Called when the user switches tabs. */
	onActiveTabChange: (tabId: string) => void;
}

/** How far (px) each chevron click scrolls the tab strip. */
const TAB_SCROLL_AMOUNT = 120;

/**
 * Tracks whether the tab scroll container overflows and
 * exposes scroll helpers for the chevron buttons.
 */
function useTabScroll() {
	const ref = useRef<HTMLDivElement>(null);
	const [canScrollLeft, setCanScrollLeft] = useState(false);
	const [canScrollRight, setCanScrollRight] = useState(false);

	useEffect(() => {
		const el = ref.current;
		if (!el) return;

		const update = () => {
			setCanScrollLeft(el.scrollLeft > 0);
			setCanScrollRight(el.scrollLeft + el.clientWidth < el.scrollWidth - 1);
		};

		// Initial check.
		update();

		// Re-check on scroll.
		el.addEventListener("scroll", update, { passive: true });

		// Re-check when the container or its children resize.
		const ro = new ResizeObserver(update);
		ro.observe(el);

		return () => {
			el.removeEventListener("scroll", update);
			ro.disconnect();
		};
	}, []);

	const scrollLeft = () => {
		ref.current?.scrollBy({
			left: -TAB_SCROLL_AMOUNT,
			behavior: "smooth",
		});
	};

	const scrollRight = () => {
		ref.current?.scrollBy({
			left: TAB_SCROLL_AMOUNT,
			behavior: "smooth",
		});
	};

	return { ref, canScrollLeft, canScrollRight, scrollLeft, scrollRight };
}

export const SidebarTabView: FC<SidebarTabViewProps> = ({
	tabs,
	isExpanded,
	onToggleExpanded,
	isSidebarCollapsed,
	onToggleSidebarCollapsed,
	chatTitle,
	onClose,
	desktopChatId,
	activeTabId,
	onActiveTabChange,
}) => {
	const tabIdPrefix = useId();
	// Build the full list of tab IDs including the desktop tab
	// so that effectiveTabId validation covers it.
	const allTabIds = new Set(tabs.map((t) => t.id));
	if (desktopChatId) {
		allTabIds.add("desktop");
	}

	// Derive the effective tab. Fall back to the first tab if
	// the stored activeTabId no longer matches any tab in the list.
	const effectiveTabId =
		activeTabId !== null && allTabIds.has(activeTabId)
			? activeTabId
			: tabs.length > 0
				? tabs[0].id
				: desktopChatId
					? "desktop"
					: null;

	// Unified list of panels for rendering. Includes the desktop
	// tab when available so we don't need to special-case it.
	const allPanels: { id: string; content: ReactNode }[] = tabs.map((t) => ({
		id: t.id,
		content: t.content,
	}));
	if (desktopChatId) {
		allPanels.push({
			id: "desktop",
			content: (
				<DesktopPanel
					chatId={desktopChatId}
					isVisible={effectiveTabId === "desktop"}
				/>
			),
		});
	}

	const {
		ref: tabScrollRef,
		canScrollLeft,
		canScrollRight,
		scrollLeft: scrollTabsLeft,
		scrollRight: scrollTabsRight,
	} = useTabScroll();

	if (tabs.length === 0 && !desktopChatId) {
		return (
			<div className="flex h-full min-w-0 flex-col overflow-hidden bg-surface-primary">
				{/* Tab bar – always visible for the expand button. */}
				<div
					role="tablist"
					className="flex shrink-0 items-center gap-2 border-0 border-b border-solid border-border-default px-3 py-1"
				>
					<div className="min-w-0 shrink-0 text-center">
						{isExpanded && chatTitle && (
							<span className="truncate text-sm text-content-primary">
								{chatTitle}
							</span>
						)}
					</div>
					{onClose && (
						<Button
							variant="subtle"
							size="icon"
							onClick={onClose}
							aria-label="Close panel"
							className="h-7 w-7 shrink-0 text-content-secondary hover:text-content-primary lg:hidden"
						>
							<XIcon />
						</Button>
					)}
					<Button
						variant="subtle"
						size="icon"
						onClick={onToggleExpanded}
						aria-label={isExpanded ? "Collapse panel" : "Expand panel"}
						className="hidden h-7 w-7 shrink-0 text-content-secondary hover:text-content-primary lg:inline-flex"
					>
						{isExpanded ? <MinimizeIcon /> : <MaximizeIcon />}
					</Button>
				</div>
				<div className="flex flex-1 items-center justify-center p-6 text-center text-xs text-content-secondary">
					No panels available.
				</div>
			</div>
		);
	}

	return (
		<div className="flex h-full min-w-0 flex-col overflow-hidden bg-surface-primary">
			{/* Tab bar */}
			<div
				role="tablist"
				className="relative flex shrink-0 items-center gap-2 border-0 border-b border-solid border-border-default px-3 py-1"
			>
				{/* Sidebar toggle – only when expanded and sidebar is collapsed */}
				{isExpanded && isSidebarCollapsed && onToggleSidebarCollapsed && (
					<Button
						variant="subtle"
						size="icon"
						onClick={onToggleSidebarCollapsed}
						aria-label="Expand sidebar"
						className="mr-1 h-7 w-7 shrink-0"
					>
						<PanelLeftIcon />
					</Button>
				)}
				{/* Scrollable tab strip with overlay chevrons */}
				<div className="relative min-w-0 flex-1">
					{canScrollLeft && (
						<button
							type="button"
							onClick={scrollTabsLeft}
							aria-label="Scroll tabs left"
							className="absolute left-0 top-0 z-10 flex h-full w-8 cursor-pointer items-center justify-start border-none p-0 pl-1 text-content-primary [background:linear-gradient(to_right,hsl(var(--surface-primary))_50%,transparent)]"
						>
							<ChevronLeftIcon className="size-3.5" />
						</button>
					)}
					<div
						ref={tabScrollRef}
						className="flex w-full items-center gap-1 overflow-x-auto [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
					>
						{tabs.map((tab) => {
							const isActive = effectiveTabId === tab.id;
							return (
								<Button
									key={tab.id}
									id={`${tabIdPrefix}-tab-${tab.id}`}
									role="tab"
									aria-selected={isActive}
									onClick={() => onActiveTabChange(tab.id)}
									variant="outline"
									size="lg"
									className={cn(
										"shrink-0 h-6 min-w-0 gap-1.5 px-2 py-0 bg-surface-primary",
										isActive &&
											"bg-surface-quaternary/25 text-content-primary hover:bg-surface-quaternary/50",
										tab.badge && "pr-0",
									)}
								>
									{tab.icon}
									{tab.label}
									{tab.badge && (
										<span
											className={cn(
												"flex -my-px items-center self-stretch transition-opacity",
												!isActive && "opacity-50",
											)}
										>
											{tab.badge}
										</span>
									)}
								</Button>
							);
						})}
						{desktopChatId && (
							<Button
								id={`${tabIdPrefix}-tab-desktop`}
								role="tab"
								aria-selected={effectiveTabId === "desktop"}
								onClick={() => onActiveTabChange("desktop")}
								variant="outline"
								size="lg"
								className={cn(
									"shrink-0 h-6 min-w-0 gap-1.5 px-2 py-0 bg-surface-primary",
									effectiveTabId === "desktop" &&
										"bg-surface-quaternary/25 text-content-primary hover:bg-surface-quaternary/50",
								)}
							>
								Desktop
							</Button>
						)}
					</div>
					{canScrollRight && (
						<button
							type="button"
							onClick={scrollTabsRight}
							aria-label="Scroll tabs right"
							className="absolute right-0 top-0 z-10 flex h-full w-8 cursor-pointer items-center justify-end border-none p-0 pr-1 text-content-primary [background:linear-gradient(to_left,hsl(var(--surface-primary))_50%,transparent)]"
						>
							<ChevronRightIcon className="size-3.5" />
						</button>
					)}
				</div>
				{/* Center: chat title when expanded */}
				{isExpanded && chatTitle && (
					<div className="pointer-events-none absolute inset-0 flex items-center justify-center">
						<span className="truncate px-24 text-sm text-content-primary">
							{chatTitle}
						</span>
					</div>
				)}
				{/* Right side: close (mobile) / expand (desktop) */}
				{onClose && (
					<Button
						variant="subtle"
						size="icon"
						onClick={onClose}
						aria-label="Close panel"
						className="h-7 w-7 shrink-0 text-content-secondary hover:text-content-primary lg:hidden"
					>
						<XIcon />
					</Button>
				)}
				<Button
					variant="subtle"
					size="icon"
					onClick={onToggleExpanded}
					aria-label={isExpanded ? "Collapse panel" : "Expand panel"}
					className="hidden h-7 w-7 shrink-0 text-content-secondary hover:text-content-primary lg:inline-flex"
				>
					{isExpanded ? <MinimizeIcon /> : <MaximizeIcon />}
				</Button>
			</div>
			{/* Tab panels – all stay mounted, only the active one visible. */}
			{allPanels.map((panel) => {
				const isActive = effectiveTabId === panel.id;
				return (
					<div
						key={panel.id}
						role="tabpanel"
						aria-labelledby={`${tabIdPrefix}-tab-${panel.id}`}
						className={cn("min-h-0 flex-1", !isActive && "hidden")}
						inert={!isActive}
					>
						{panel.content}
					</div>
				);
			})}
		</div>
	);
};
