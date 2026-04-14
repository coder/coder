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

	const handleMouseMove = useCallback((e: React.MouseEvent) => {
		const rect = containerRef.current?.getBoundingClientRect();
		if (rect) {
			setMouseY(e.clientY - rect.top);
		}
	}, []);

	const handleMouseLeave = useCallback(() => {
		setMouseY(null);
	}, []);

	// Build a radial gradient mask centered on the cursor so the
	// glow fades to transparent 150px above and below.
	const glowStyle =
		mouseY !== null
			? {
					maskImage: `linear-gradient(to bottom, transparent ${mouseY - 150}px, white ${mouseY - 80}px, white ${mouseY + 80}px, transparent ${mouseY + 150}px)`,
					WebkitMaskImage: `linear-gradient(to bottom, transparent ${mouseY - 150}px, white ${mouseY - 80}px, white ${mouseY + 80}px, transparent ${mouseY + 150}px)`,
				}
			: {
					maskImage: "none",
					WebkitMaskImage: "none",
				};

	return (
		<div
			ref={containerRef}
			role="separator"
			aria-label="Resize sidebar"
			aria-valuenow={0}
			tabIndex={0}
			onPointerDown={onDragStart}
			onMouseMove={handleMouseMove}
			onMouseLeave={handleMouseLeave}
			className={cn(
				// Centered on the 1px border at the nav's right edge.
				"absolute top-0 -right-1 h-full w-2 cursor-col-resize z-10",
				"flex items-center justify-center",
				"group",
			)}
		>
			<div
				className={cn(
					"h-full w-[2px] rounded-full",
					"bg-content-secondary opacity-0",
					"transition-opacity duration-150",
					"group-hover:opacity-40 group-active:opacity-70",
				)}
				style={glowStyle}
			/>
		</div>
	);
};
