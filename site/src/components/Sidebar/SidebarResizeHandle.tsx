import { type FC, useCallback, useRef, useState } from "react";
import { cn } from "#/utils/cn";

interface SidebarResizeHandleProps {
	onDragStart: (e: React.PointerEvent) => undefined | (() => void);
}

/**
 * An invisible hit area on the sidebar's right edge. On hover,
 * a 2px line appears spanning the full height of the border.
 * The line stays visible while dragging.
 */
export const SidebarResizeHandle: FC<SidebarResizeHandleProps> = ({
	onDragStart,
}) => {
	const containerRef = useRef<HTMLDivElement>(null);
	const [hovered, setHovered] = useState(false);
	const [dragging, setDragging] = useState(false);

	const handleMouseEnter = useCallback(() => {
		setHovered(true);
	}, []);

	const handleMouseLeave = useCallback(() => {
		setHovered(false);
	}, []);

	const handlePointerDown = useCallback(
		(e: React.PointerEvent) => {
			setDragging(true);

			const onPointerUp = () => {
				setDragging(false);
				setHovered(false);
				document.removeEventListener("pointerup", onPointerUp);
			};

			document.addEventListener("pointerup", onPointerUp);
			onDragStart(e);
		},
		[onDragStart],
	);

	const visible = hovered || dragging;

	return (
		<div
			ref={containerRef}
			role="separator"
			aria-label="Resize sidebar"
			aria-valuenow={0}
			tabIndex={0}
			onPointerDown={handlePointerDown}
			onMouseEnter={handleMouseEnter}
			onMouseLeave={handleMouseLeave}
			className={cn(
				"absolute top-0 -right-2 h-full w-4 cursor-col-resize z-10",
			)}
		>
			<div
				className="absolute top-0 h-full w-[2px] rounded-full bg-border"
					style={{
						left: 7,
						opacity: visible ? 1 : 0,
						transition: "opacity 150ms",
					}}			/>
		</div>
	);
};
