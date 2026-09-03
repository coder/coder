import { type FC, type ReactNode, useMemo } from "react";
import { cn } from "#/utils/cn";
import { SidebarContext } from "./SidebarContext";
import { SidebarResizeHandle } from "./SidebarResizeHandle";
import { useSidebarResize } from "./useSidebarResize";

interface CollapsibleSidebarProps {
	children: ReactNode;
	className?: string;
	storageKey?: string;
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

	const nav = (
		<nav
			className={cn(
				"h-full w-[240px] overflow-y-auto",
				"flex flex-col",
				"px-3 pt-6 pb-6",
				className,
			)}
		>
			{children}
		</nav>
	);

	return (
		<SidebarContext.Provider value={contextValue}>
			{/* Non-clipping wrapper for positioning. The resize handle
			    lives here so it isn't clipped by overflow-hidden. */}
			<div
				data-sidebar-container
				className="relative flex-shrink-0 sticky top-0 h-screen z-30 transition-[width] duration-150 ease-in-out"
				style={{ width }}
			>
				{peeking ? (
					// The container already holds only the rail width in
					// layout flow; the expanded nav floats over the content.
					<div className="absolute left-0 top-0 h-full w-[240px] z-20 overflow-hidden bg-surface-primary border-0 border-r border-solid border-border shadow-lg">
						{nav}
					</div>
				) : (
					<>
						{/* Clipping container for the nav content. */}
						<div className="h-full overflow-hidden">{nav}</div>
						{/* Handle sits outside the overflow-hidden div so
						    its right half isn't clipped. */}
						<SidebarResizeHandle onDragStart={onDragStart} />
					</>
				)}
			</div>
		</SidebarContext.Provider>
	);
};
