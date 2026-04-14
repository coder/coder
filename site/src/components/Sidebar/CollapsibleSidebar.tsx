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
	const { width, collapsed, dragging, expand, onDragStart } =
		useSidebarResize(storageKey);

	const contextValue = useMemo(
		() => ({ collapsed, expand }),
		[collapsed, expand],
	);

	return (
		<SidebarContext.Provider value={contextValue}>
			<div
				data-sidebar-container
				className={cn(
					"relative flex-shrink-0 sticky top-0 h-screen",
					// Disable transition during drag so the 1:1 lead-in
					// tracks the cursor without lag. Re-enable after
					// release so the snap animation plays.
					!dragging && "transition-[width] duration-150 ease-in-out",
				)}
				style={{ width }}
			>
				<nav
					className={cn(
						"h-full overflow-y-auto overflow-x-hidden",
						"flex flex-col border-0 border-r border-solid border-border",
						// Nav px-3 + button px-3 = 24px on each side,
						// matching navbar px-6 for icon alignment.
						"px-3 pt-6 pb-6",
						className,
					)}
				>
					{children}
				</nav>
				<SidebarResizeHandle onDragStart={onDragStart} />
			</div>
		</SidebarContext.Provider>
	);
};
