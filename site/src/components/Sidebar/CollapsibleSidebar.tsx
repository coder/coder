import { type FC, type ReactNode, useEffect, useMemo, useRef } from "react";
import { cn } from "#/utils/cn";
import { SidebarContext } from "./SidebarContext";
import { SidebarResizeHandle } from "./SidebarResizeHandle";
import { useSidebarResize } from "./useSidebarResize";

interface CollapsibleSidebarProps {
	children: ReactNode;
	className?: string;
	storageKey?: string;
	/**
	 * When set, the sidebar only occupies its collapsed width in
	 * layout flow; the expanded panel renders as a flyout over the
	 * adjacent content instead of pushing it. Clicking outside the
	 * panel or pressing Escape collapses it.
	 */
	overlay?: boolean;
	/**
	 * When set, briefly expand the sidebar on mount to show the menu,
	 * then auto-collapse. Skipped when the user previously left the
	 * sidebar expanded. Only meaningful together with overlay.
	 */
	peekOnMount?: boolean;
}

export const CollapsibleSidebar: FC<CollapsibleSidebarProps> = ({
	children,
	className,
	storageKey = "sidebar-width",
	overlay = false,
	peekOnMount = false,
}) => {
	const {
		width,
		collapsed,
		expand,
		collapse,
		toggle,
		cancelPeek,
		onDragStart,
	} = useSidebarResize(storageKey, { peekOnMount });
	const containerRef = useRef<HTMLDivElement>(null);

	const contextValue = useMemo(
		() => ({ collapsed, expand, collapse, toggle }),
		[collapsed, expand, collapse, toggle],
	);

	// In overlay mode the expanded panel covers content, so dismiss it
	// when the user clicks outside or presses Escape. Listeners are
	// only attached while the overlay is expanded.
	useEffect(() => {
		if (!overlay || collapsed) {
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
	}, [overlay, collapsed, collapse]);

	if (overlay) {
		return (
			<SidebarContext.Provider value={contextValue}>
				{/* The container holds only the collapsed rail width in
				    layout flow; the panel expands over the content. */}
				<div
					ref={containerRef}
					data-sidebar-container
					className="relative flex-shrink-0 sticky top-0 h-screen w-16"
					onPointerDown={cancelPeek}
				>
					<div
						className={cn(
							"absolute left-0 top-0 h-full z-20 overflow-hidden",
							"bg-surface-primary transition-[width] duration-150 ease-in-out",
							!collapsed &&
								"border-0 border-r border-solid border-border shadow-lg",
						)}
						style={{ width }}
					>
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
					</div>
				</div>
			</SidebarContext.Provider>
		);
	}

	return (
		<SidebarContext.Provider value={contextValue}>
			{/* Non-clipping wrapper for positioning. The resize handle
			    lives here so it isn't clipped by overflow-hidden. */}
			<div
				data-sidebar-container
				className="relative flex-shrink-0 sticky top-0 h-screen transition-[width] duration-150 ease-in-out"
				style={{ width }}
			>
				{/* Clipping container for the nav content. */}
				<div className="h-full overflow-hidden">
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
				</div>
				{/* Handle sits outside the overflow-hidden div so
				    its right half isn't clipped. */}
				<SidebarResizeHandle onDragStart={onDragStart} />
			</div>
		</SidebarContext.Provider>
	);
};
