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
				"absolute top-0 right-0 h-full w-2 cursor-col-resize",
				"flex items-center justify-center",
				"group",
			)}
		>
			{/* Visible indicator line — uses a gradient mask to fade out at
			    the top and bottom so it doesn't create hard visual edges. */}
			<div
				className={cn(
					"h-full w-[3px] rounded-full",
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
