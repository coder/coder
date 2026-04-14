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
			{/* Outer container controls the visible width and clips
			    overflow. The inner nav always renders at full expanded
			    width so content never reflows during drag — it just
			    gets cropped. */}
			<div
				data-sidebar-container
				className="relative flex-shrink-0 sticky top-0 h-screen overflow-hidden transition-[width] duration-150 ease-in-out"
				style={{ width }}
			>
				<nav
					className={cn(
						"h-full w-[240px] overflow-y-auto",
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
