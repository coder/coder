import { type FC, useCallback, useRef, useState } from "react";
import { cn } from "#/utils/cn";

interface SidebarResizeHandleProps {
	onDragStart: (e: React.PointerEvent) => undefined | (() => void);
}

/**
 * An invisible 8px hit area on the sidebar's right edge. On hover,
 * a 2px glow line appears centered on the cursor (300px tall, 150px
 * above and below) rather than spanning the full height.
 */
export const SidebarResizeHandle: FC<SidebarResizeHandleProps> = ({
	onDragStart,
}) => {
	const containerRef = useRef<HTMLDivElement>(null);
	const [mouseY, setMouseY] = useState<number | null>(null);
	const [hovered, setHovered] = useState(false);

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
		setMouseY(null);
	}, []);

	// Build a gradient mask centered on the cursor. When the mouse
	// isn't hovering, use a fully transparent mask so there's no
	// flash when leaving (avoids the brief full-line flicker that
	// occurs if we switch to mask:none while opacity transitions).
	const maskGradient =
		mouseY !== null
			? `linear-gradient(to bottom, transparent ${mouseY - 150}px, white ${mouseY - 80}px, white ${mouseY + 80}px, transparent ${mouseY + 150}px)`
			: "linear-gradient(transparent, transparent)";

	return (
		<div
			ref={containerRef}
			role="separator"
			aria-label="Resize sidebar"
			aria-valuenow={0}
			tabIndex={0}
			onPointerDown={onDragStart}
			onMouseMove={handleMouseMove}
			onMouseEnter={handleMouseEnter}
			onMouseLeave={handleMouseLeave}
			className={cn(
				// Centered on the 1px border at the nav's right edge.
				"absolute top-0 -right-1 h-full w-2 cursor-col-resize z-10",
				"flex items-center justify-center",
			)}
		>
			<div
				className="h-full w-[2px] rounded-full bg-content-secondary"
				style={{
					opacity: hovered ? 0.4 : 0,
					transition: "opacity 150ms",
					maskImage: maskGradient,
					WebkitMaskImage: maskGradient,
				}}
			/>
		</div>
	);
};
