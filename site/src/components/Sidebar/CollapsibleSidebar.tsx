import { type FC, type ReactNode, useMemo, useRef } from "react";
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
	const dragging = useRef(false);
	const { width, collapsed, onDragStart } = useSidebarResize(storageKey);

	const contextValue = useMemo(() => ({ collapsed }), [collapsed]);

	const handleDragStart = (e: React.PointerEvent) => {
		dragging.current = true;

		const cleanup = onDragStart(e);

		// Restore the transition once the drag ends so subsequent
		// programmatic width changes still animate smoothly.
		const onUp = () => {
			dragging.current = false;
			document.removeEventListener("pointerup", onUp);
		};
		document.addEventListener("pointerup", onUp);

		return cleanup;
	};

	return (
		<SidebarContext.Provider value={contextValue}>
			<nav
				style={{ width }}
				className={cn(
					"relative flex-shrink-0 overflow-hidden pl-6 pt-6 pb-6",
					!dragging.current && "transition-[width] duration-150 ease-in-out",
					className,
				)}
			>
				{children}
				<SidebarResizeHandle onDragStart={handleDragStart} />
			</nav>
		</SidebarContext.Provider>
	);
};
