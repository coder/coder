import { Button } from "components/Button/Button";
import {
	ChevronLeftIcon,
	ChevronRightIcon,
	MaximizeIcon,
	MinimizeIcon,
	PanelLeftIcon,
	XIcon,
} from "lucide-react";
import type { ReactNode } from "react";
import {
	type FC,
	useCallback,
	useEffect,
	useId,
	useRef,
	useState,
} from "react";
import { cn } from "utils/cn";

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

	const update = useCallback(() => {
		const el = ref.current;
		if (!el) return;
		setCanScrollLeft(el.scrollLeft > 0);
		setCanScrollRight(el.scrollLeft + el.clientWidth < el.scrollWidth - 1);
	}, []);

	useEffect(() => {
		const el = ref.current;
		if (!el) return;

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
	}, [update]);

	const scrollLeft = useCallback(() => {
		ref.current?.scrollBy({
			left: -TAB_SCROLL_AMOUNT,
			behavior: "smooth",
		});
	}, []);

	const scrollRight = useCallback(() => {
		ref.current?.scrollBy({
			left: TAB_SCROLL_AMOUNT,
			behavior: "smooth",
		});
	}, []);

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
}) => {
	const tabIdPrefix = useId();
	const [activeTabId, setActiveTabId] = useState<string | null>(
		tabs.length > 0 ? tabs[0].id : null,
	);

	// Derive the effective tab. Fall back to the first tab if
	// the stored activeTabId no longer matches any tab in the list.
	const effectiveTabId =
		activeTabId !== null && tabs.some((t) => t.id === activeTabId)
			? activeTabId
			: tabs.length > 0
				? tabs[0].id
				: null;

	const activeTab = tabs.find((t) => t.id === effectiveTabId) ?? null;

	const tabScroll = useTabScroll();

	if (tabs.length === 0) {
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
							className="h-7 w-7 shrink-0 text-content-secondary hover:text-content-primary md:hidden"
						>
							<XIcon />
						</Button>
					)}
					<Button
						variant="subtle"
						size="icon"
						onClick={onToggleExpanded}
						aria-label={isExpanded ? "Collapse panel" : "Expand panel"}
						className="hidden h-7 w-7 shrink-0 text-content-secondary hover:text-content-primary md:inline-flex"
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
					{tabScroll.canScrollLeft && (
						<button
							type="button"
							onClick={tabScroll.scrollLeft}
							aria-label="Scroll tabs left"
							className="absolute left-0 top-0 z-10 flex h-full w-8 cursor-pointer items-center justify-start border-none p-0 pl-1 text-content-primary [background:linear-gradient(to_right,hsl(var(--surface-primary))_50%,transparent)]"
						>
							<ChevronLeftIcon className="size-3.5" />
						</button>
					)}
					<div
						ref={tabScroll.ref}
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
									onClick={() => setActiveTabId(tab.id)}
									variant="subtle"
									size="lg"
									className={cn(
										"shrink-0 h-6 border border-solid border-transparent min-w-0 gap-3 px-2 py-0 bg-surface-primary text-content-secondary hover:bg-surface-tertiary/50 hover:text-content-primary",
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
					</div>
					{tabScroll.canScrollRight && (
						<button
							type="button"
							onClick={tabScroll.scrollRight}
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
						className="h-7 w-7 shrink-0 text-content-secondary hover:text-content-primary md:hidden"
					>
						<XIcon />
					</Button>
				)}
				<Button
					variant="subtle"
					size="icon"
					onClick={onToggleExpanded}
					aria-label={isExpanded ? "Collapse panel" : "Expand panel"}
					className="hidden h-7 w-7 shrink-0 text-content-secondary hover:text-content-primary md:inline-flex"
				>
					{isExpanded ? <MinimizeIcon /> : <MaximizeIcon />}
				</Button>
			</div>
			{/* Tab content */}
			<div
				role="tabpanel"
				aria-labelledby={
					effectiveTabId ? `${tabIdPrefix}-tab-${effectiveTabId}` : undefined
				}
				className="min-h-0 flex-1"
			>
				{activeTab?.content}
			</div>
		</div>
	);
};
