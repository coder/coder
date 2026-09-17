import { cn } from "cn";
import type { ComponentPropsWithRef, FC } from "react";

type IconButtonProps = ComponentPropsWithRef<"button"> & {
	readonly "aria-label": string;
};

/**
 * The size-4 icon control used in card and column headers: the actions
 * trigger, the chat opener, the column's new-chat button. Those headers
 * are drag handles, so the press never propagates and cannot start a drag;
 * `z-[1]` keeps it above a card's open-chat surface. Spreads the rest so
 * Radix can use it as a trigger with `asChild`.
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
