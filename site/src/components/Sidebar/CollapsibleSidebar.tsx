import { type FC, type ReactNode, useMemo } from "react";
import { cn } from "#/utils/cn";
import { SidebarContext } from "./SidebarContext";
import { SidebarResizeHandle } from "./SidebarResizeHandle";
import { useSidebarResize } from "./useSidebarResize";

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
	const { width, collapsed, peeking, expand, toggle, onDragStart } =
		useSidebarResize(storageKey, { preferCollapsed });

	// During a peek the layout is collapsed but the nav renders its
	// expanded content inside the flyout.
	const contextValue = useMemo(
		() => ({ collapsed: collapsed && !peeking, expand, toggle }),
		[collapsed, peeking, expand, toggle],
	);

	// The header stays put; only the nav list scrolls, within its own
	// scroll area so the page scrollbar never moves the sidebar.
	const panel = (
		<div className="flex h-full w-[240px] flex-col">
			{header}
			<nav
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
				data-sidebar-container
				className="relative shrink-0 sticky z-30 transition-[width] duration-150 ease-in-out"
				style={{
					width,
					top: NAVBAR_HEIGHT,
					height: `calc(100vh - ${NAVBAR_HEIGHT + bottomInset}px)`,
				}}
			>
				{peeking ? (
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
						<SidebarResizeHandle onDragStart={onDragStart} />
					</>
				)}
			</div>
		</SidebarContext.Provider>
	);
};
