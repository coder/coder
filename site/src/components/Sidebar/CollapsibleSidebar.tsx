import { cn } from "cn";
import { type FC, type ReactNode, useEffect, useMemo, useRef } from "react";
import { SidebarContext } from "./SidebarContext";
import { SidebarResizeHandle } from "./SidebarResizeHandle";
import { COLLAPSED_WIDTH, useSidebarResize } from "./useSidebarResize";

/** Height of the sticky dashboard navbar the sidebar sits beneath. */
const NAVBAR_HEIGHT = 72;

interface CollapsibleSidebarProps {
	children: ReactNode;
	className?: string;
	storageKey?: string;
	/**
	 * Content pinned above the scrolling nav list, inside the collapsing
	 * clipper. Receives the sidebar context, so it can render its own
	 * collapsed variant.
	 */
	header?: ReactNode;
	/**
	 * Space in px to leave free at the bottom of the viewport, for a
	 * bottom-pinned bar such as the deployment banner.
	 */
	bottomInset?: number;
}

/**
 * Sticky sidebar column beneath the dashboard navbar that collapses to a
 * 64px icon rail. The width is persisted per `storageKey`, narrow
 * viewports start collapsed, and below the md breakpoint the expanded
 * state is a drawer over the page. Children read `SidebarContext` to
 * render their collapsed variant.
 */
export const CollapsibleSidebar: FC<CollapsibleSidebarProps> = ({
	children,
	className,
	storageKey = "sidebar-width",
	header,
	bottomInset = 0,
}) => {
	const { width, collapsed, mobile, expand, collapse, toggle, onDragStart } =
		useSidebarResize(storageKey);
	const containerRef = useRef<HTMLDivElement>(null);

	// Below md the expanded sidebar is a full-width drawer over the page
	// while the icon rail keeps its place in the layout underneath.
	const drawerOpen = mobile && !collapsed;

	const contextValue = useMemo(
		() => ({ collapsed, expand, toggle }),
		[collapsed, expand, toggle],
	);

	// The drawer covers the page, so dismiss it when the user clicks
	// outside or presses Escape. Listeners only exist while it is open.
	useEffect(() => {
		if (!drawerOpen) {
			return;
		}
		const handlePointerDown = (event: PointerEvent) => {
			const container = containerRef.current;
			if (container && !container.contains(event.target as Node)) {
				collapse();
			}
		};
		const handleKeyDown = (event: KeyboardEvent) => {
			if (event.key === "Escape") {
				collapse();
			}
		};
		document.addEventListener("pointerdown", handlePointerDown);
		document.addEventListener("keydown", handleKeyDown);
		return () => {
			document.removeEventListener("pointerdown", handlePointerDown);
			document.removeEventListener("keydown", handleKeyDown);
		};
	}, [drawerOpen, collapse]);

	// Following a link closes the drawer. Delegated on the nav element so
	// views need no wiring; keyboard activation of a link dispatches a
	// click as well.
	const navRef = useRef<HTMLElement>(null);
	useEffect(() => {
		const nav = navRef.current;
		if (!drawerOpen || !nav) {
			return;
		}
		const handleClick = (event: MouseEvent) => {
			if ((event.target as HTMLElement).closest("a[href]")) {
				collapse();
			}
		};
		nav.addEventListener("click", handleClick);
		return () => nav.removeEventListener("click", handleClick);
	}, [drawerOpen, collapse]);

	// The header stays put; only the nav list scrolls, within its own
	// scroll area so the page scrollbar never moves the sidebar.
	const panel = (
		<div
			data-testid="sidebar-panel"
			className={cn(
				"flex h-full flex-col",
				drawerOpen ? "w-full" : "w-[240px]",
			)}
		>
			{/* Both the header and the list reserve a stable scrollbar gutter
			    so the toggle stays put when the list starts to scroll (only
			    matters with classic, non-overlay scrollbars). */}
			<div className="overflow-hidden [scrollbar-gutter:stable]">{header}</div>
			{header && <div className="h-px shrink-0 bg-border" />}
			<nav
				ref={navRef}
				data-testid="sidebar-scroll-area"
				className={cn(
					"flex-1 min-h-0 overflow-y-auto [scrollbar-gutter:stable]",
					"flex flex-col",
					"px-3 pt-2 pb-6",
					className,
				)}
			>
				{children}
			</nav>
		</div>
	);

	return (
		<SidebarContext.Provider value={contextValue}>
			{/* Non-clipping wrapper for positioning. The resize handle
			    lives here so it isn't clipped by overflow-hidden. */}
			<div
				ref={containerRef}
				data-sidebar-container
				className="relative shrink-0 sticky z-30 transition-[width] duration-150 ease-in-out"
				style={{
					width: drawerOpen ? COLLAPSED_WIDTH : width,
					top: NAVBAR_HEIGHT,
					height: `calc(100vh - ${NAVBAR_HEIGHT + bottomInset}px)`,
				}}
			>
				{drawerOpen ? (
					<div
						className="fixed inset-x-0 z-30 overflow-hidden bg-surface-primary border-0 border-b border-solid border-border shadow-lg"
						style={{ top: NAVBAR_HEIGHT, bottom: bottomInset }}
					>
						{panel}
					</div>
				) : (
					<>
						{/* Clipping container for the nav content. */}
						<div className="h-full overflow-hidden">{panel}</div>
						{/* Handle sits outside the overflow-hidden div so
						    its right half isn't clipped. */}
						{!mobile && <SidebarResizeHandle onDragStart={onDragStart} />}
					</>
				)}
			</div>
		</SidebarContext.Provider>
	);
};
