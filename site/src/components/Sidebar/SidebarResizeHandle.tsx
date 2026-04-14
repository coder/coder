import { type FC, useCallback, useRef, useState } from "react";
import { cn } from "#/utils/cn";

interface SidebarResizeHandleProps {
	onDragStart: (e: React.PointerEvent) => undefined | (() => void);
}

/**
 * An invisible hit area on the sidebar's right edge. On hover,
 * a 2px glow line appears centered on the cursor (300px tall).
 * The glow stays visible while dragging even if the cursor
 * leaves the handle area.
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

			// Track cursor position globally during drag so the
			// glow line follows even outside the handle element.
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

	const maskGradient =
		mouseY !== null
			? `linear-gradient(to bottom, transparent ${mouseY - 150}px, white ${mouseY - 80}px, white ${mouseY + 80}px, transparent ${mouseY + 150}px)`
			: "linear-gradient(transparent, transparent)";

	const visible = hovered || dragging;

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
				// 8px buffer on each side of the 1px border line.
				"absolute top-0 -right-2 h-full w-4 cursor-col-resize z-10",
				"flex items-center justify-center",
			)}
		>
			<div
				className="h-full w-[2px] rounded-full bg-content-secondary"
				style={{
					opacity: visible ? (dragging ? 0.7 : 0.4) : 0,
					transition: "opacity 150ms",
					maskImage: maskGradient,
					WebkitMaskImage: maskGradient,
				}}
			/>
		</div>
	);
};
