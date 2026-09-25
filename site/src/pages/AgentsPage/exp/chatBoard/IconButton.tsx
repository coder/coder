import { cn } from "cn";
import type { ComponentProps, FC } from "react";

type IconButtonProps = ComponentProps<"button"> & {
	readonly "aria-label": string;
};

/**
 * Stops pointerdown so a press in a drag-handle header cannot start a drag;
 * `z-[1]` keeps it above a card's open-chat surface. Spreads props so Radix
 * `asChild` works.
 */
export const IconButton: FC<IconButtonProps> = ({
	className,
	onPointerDown,
	children,
	...props
}) => (
	<button
		type="button"
		className={cn(
			"relative z-[1] grid size-4 shrink-0 place-items-center rounded border-0 bg-transparent p-0 text-content-secondary/60 hover:text-content-primary",
			className,
		)}
		onPointerDown={(e) => {
			e.stopPropagation();
			onPointerDown?.(e);
		}}
		{...props}
	>
		{children}
	</button>
);
