import { type FC, type ReactNode, useMemo } from "react";
import { cn } from "#/utils/cn";
import { SidebarContext } from "./SidebarContext";
import { SidebarResizeHandle } from "./SidebarResizeHandle";
import { useSidebarResize } from "./useSidebarResize";

interface CollapsibleSidebarProps {
	children: ReactNode;
	className?: string;
	storageKey?: string;
}

export const CollapsibleSidebar: FC<CollapsibleSidebarProps> = ({
	children,
	className,
	storageKey = "sidebar-width",
}) => {
	const { width, collapsed, expand, onDragStart } =
		useSidebarResize(storageKey);

	const contextValue = useMemo(
		() => ({ collapsed, expand }),
		[collapsed, expand],
	);

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
				<div className="h-full overflow-hidden border-0 border-r border-solid border-border">
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
