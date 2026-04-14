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
	const { width, collapsed, onDragStart } = useSidebarResize(storageKey);

	const contextValue = useMemo(() => ({ collapsed }), [collapsed]);

	return (
		<SidebarContext.Provider value={contextValue}>
			{/* Outer wrapper holds both the scrollable nav and the
			    resize handle. The handle sits outside the nav's
			    overflow so it's never clipped. */}
			<div
				className="relative flex-shrink-0 sticky top-0 h-screen"
				style={{ width }}
			>
				<nav
					className={cn(
						"h-full overflow-y-auto overflow-x-hidden",
						"flex flex-col border-0 border-r border-solid border-border",
						// pl-6 (24px) matches navbar px-6 so the icon left
						// edges align with the Coder logo left edge.
						"pl-6 pt-6 pb-6 pr-6",
						"transition-[width] duration-150 ease-in-out",
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
