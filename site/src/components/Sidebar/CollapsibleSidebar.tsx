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
			<div
				data-sidebar-container
				className="relative flex-shrink-0 sticky top-0 h-screen transition-[width] duration-150 ease-in-out"
				style={{ width }}
			>
				<nav
					className={cn(
						"h-full overflow-y-auto overflow-x-hidden",
						"flex flex-col border-0 border-r border-solid border-border",
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
