import { type FC, useCallback, useRef, useState } from "react";
import { cn } from "#/utils/cn";

interface SidebarResizeHandleProps {
	onDragStart: (e: React.PointerEvent) => undefined | (() => void);
}

/**
 * An invisible hit area on the sidebar's right edge. On hover,
 * a 2px bar (150px tall) appears centered on the cursor.
 * The bar stays visible while dragging.
 */
export const SidebarResizeHandle: FC<SidebarResizeHandleProps> = ({
	onDragStart,
}) => {
	const containerRef = useRef<HTMLDivElement>(null);
	const [mouseY, setMouseY] = useState<number | null>(null);
	const [hovered, setHovered] = useState(false);
	const [dragging, setDragging] = useState(false);

	const handleMouseMove = useCallback((e: React.MouseEvent) => {
		const rect = containerRef.current?.getBoundingClientRect();
		if (rect) {
			setMouseY(e.clientY - rect.top);
		}
	}, []);

	const handleMouseEnter = useCallback(() => {
		setHovered(true);
	}, []);

	const handleMouseLeave = useCallback(() => {
		setHovered(false);
		if (!dragging) {
			setMouseY(null);
		}
	}, [dragging]);

	const handlePointerDown = useCallback(
		(e: React.PointerEvent) => {
			setDragging(true);

			const onPointerMove = (moveEvent: PointerEvent) => {
				const rect = containerRef.current?.getBoundingClientRect();
				if (rect) {
					setMouseY(moveEvent.clientY - rect.top);
				}
			};

			const onPointerUp = () => {
				setDragging(false);
				setHovered(false);
				setMouseY(null);
				document.removeEventListener("pointermove", onPointerMove);
				document.removeEventListener("pointerup", onPointerUp);
			};

			document.addEventListener("pointermove", onPointerMove);
			document.addEventListener("pointerup", onPointerUp);

			onDragStart(e);
		},
		[onDragStart],
	);

	const visible = (hovered || dragging) && mouseY !== null;
	const BAR_HEIGHT = 150;

	return (
		<div
			ref={containerRef}
			role="separator"
			aria-label="Resize sidebar"
			aria-valuenow={0}
			tabIndex={0}
			onPointerDown={handlePointerDown}
			onMouseMove={handleMouseMove}
			onMouseEnter={handleMouseEnter}
			onMouseLeave={handleMouseLeave}
			className={cn(
				"absolute top-0 -right-2 h-full w-4 cursor-col-resize z-10",
			)}
		>
			<div
				className="absolute w-[2px] rounded-full bg-content-secondary"
				style={{
					left: 7,
					height: BAR_HEIGHT,
					top: mouseY !== null ? mouseY - BAR_HEIGHT / 2 : 0,
					opacity: visible ? (dragging ? 0.7 : 0.4) : 0,
					transition: "opacity 150ms",
				}}
			/>
		</div>
	);
};
