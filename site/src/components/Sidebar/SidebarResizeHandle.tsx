import type { FC } from "react";
import { cn } from "#/utils/cn";

interface SidebarResizeHandleProps {
	onDragStart: (e: React.PointerEvent) => undefined | (() => void);
}

export const SidebarResizeHandle: FC<SidebarResizeHandleProps> = ({
	onDragStart,
}) => {
	return (
		<div
			role="separator"
			aria-label="Resize sidebar"
			aria-valuenow={0}
			tabIndex={0}
			onPointerDown={onDragStart}
			className={cn(
				// Positioned on the outer wrapper (not inside the nav)
				// so overflow clipping doesn't hide it. Centered on the
				// border line at right:0.
				"absolute top-0 -right-1 h-full w-2 cursor-col-resize z-10",
				"flex items-center justify-center",
				"group",
			)}
		>
			{/* 2px glow line with gradient mask fading at top/bottom. */}
			<div
				className={cn(
					"h-full w-[2px] rounded-full",
					"bg-content-secondary opacity-0",
					"transition-opacity duration-150",
					"group-hover:opacity-40 group-active:opacity-70",
				)}
				style={{
					maskImage:
						"linear-gradient(to bottom, transparent, white 20%, white 80%, transparent)",
					WebkitMaskImage:
						"linear-gradient(to bottom, transparent, white 20%, white 80%, transparent)",
				}}
			/>
		</div>
	);
};
