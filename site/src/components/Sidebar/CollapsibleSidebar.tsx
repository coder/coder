import { type FC, type ReactNode, useEffect, useMemo, useRef } from "react";
import { cn } from "#/utils/cn";
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
	/**
	 * The current page has wide content. The sidebar settles collapsed
	 * (briefly peeking the expanded nav as a flyout if it was open) and
	 * restores the user's preference once this clears.
	 */
	preferCollapsed?: boolean;
}

export const CollapsibleSidebar: FC<CollapsibleSidebarProps> = ({
	children,
	className,
	storageKey = "sidebar-width",
	header,
	bottomInset = 0,
	preferCollapsed = false,
}) => {
	const {
		width,
		collapsed,
		peeking,
		mobile,
		expand,
		collapse,
		toggle,
		onDragStart,
	} = useSidebarResize(storageKey, { preferCollapsed });
	const containerRef = useRef<HTMLDivElement>(null);

	// Below md the expanded sidebar is a full-width drawer over the page
	// while the icon rail keeps its place in the layout underneath.
	const drawerOpen = mobile && !collapsed;

	// During a peek the layout is collapsed but the nav renders its
	// expanded content inside the flyout.
	const contextValue = useMemo(
		() => ({ collapsed: collapsed && !peeking, expand, toggle }),
		[collapsed, peeking, expand, toggle],
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

	// Following a leaf link closes the drawer. Accordion headers are
	// buttons, so they keep toggling their section without dismissing.
	// Delegated on the nav element so views need no wiring; keyboard
	// activation of a link dispatches a click as well.
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
			{header}
			<nav
				ref={navRef}
				data-testid="sidebar-scroll-area"
				className={cn(
					"flex-1 min-h-0 overflow-y-auto",
					"flex flex-col",
					"px-3 pt-3 pb-6",
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
				) : peeking ? (
					// The container already holds only the rail width in
					// layout flow; the expanded nav floats over the content.
					<div className="absolute left-0 top-0 h-full w-[240px] z-20 overflow-hidden bg-surface-primary border-0 border-r border-solid border-border shadow-lg">
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
