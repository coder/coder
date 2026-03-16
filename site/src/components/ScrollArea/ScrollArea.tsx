/**
 * Copied from shadc/ui on 03/05/2025
 * @see {@link https://ui.shadcn.com/docs/components/scroll-area}
 */
import * as ScrollAreaPrimitive from "@radix-ui/react-scroll-area";
import { useCallback, useRef } from "react";
import { cn } from "utils/cn";

interface ScrollAreaProps
	extends React.ComponentPropsWithRef<typeof ScrollAreaPrimitive.Root> {
	scrollBarClassName?: string;
	viewportClassName?: string;
	/** Which scrollbar(s) to show. Defaults to "vertical". */
	orientation?: "vertical" | "horizontal" | "both";
}

export const ScrollArea: React.FC<ScrollAreaProps> = ({
	className,
	scrollBarClassName,
	viewportClassName,
	orientation = "vertical",
	children,
	...props
}) => {
	const viewportRef = useRef<HTMLDivElement>(null);

	// Translate vertical wheel events into horizontal scroll when the
	// scroll area only scrolls horizontally. Without this, the mouse
	// wheel does nothing on a horizontal-only container.
	const handleWheel = useCallback(
		(e: React.WheelEvent<HTMLDivElement>) => {
			if (orientation !== "horizontal") return;
			const el = viewportRef.current;
			if (!el) return;
			// Only redirect when the user is scrolling vertically.
			if (Math.abs(e.deltaY) <= Math.abs(e.deltaX)) return;
			e.preventDefault();
			el.scrollBy({ left: e.deltaY, behavior: "smooth" });
		},
		[orientation],
	);

	return (
		<ScrollAreaPrimitive.Root
			className={cn("relative overflow-hidden", className)}
			{...props}
		>
			<ScrollAreaPrimitive.Viewport
				ref={viewportRef}
				onWheel={handleWheel}
				className={cn("h-full w-full rounded-[inherit]", viewportClassName)}
			>
				{children}
			</ScrollAreaPrimitive.Viewport>
			{(orientation === "vertical" || orientation === "both") && (
				<ScrollBar
					orientation="vertical"
					className={cn("z-10", scrollBarClassName)}
				/>
			)}
			{(orientation === "horizontal" || orientation === "both") && (
				<ScrollBar
					orientation="horizontal"
					className={cn("z-10", scrollBarClassName)}
				/>
			)}
			<ScrollAreaPrimitive.Corner />
		</ScrollAreaPrimitive.Root>
	);
};

export const ScrollBar: React.FC<
	React.ComponentPropsWithRef<typeof ScrollAreaPrimitive.ScrollAreaScrollbar>
> = ({ className, orientation = "vertical", ...props }) => {
	return (
		<ScrollAreaPrimitive.ScrollAreaScrollbar
			orientation={orientation}
			className={cn(
				"border-0 border-solid border-border flex touch-none select-none transition-colors",
				orientation === "vertical" &&
					"h-full w-2.5 border-l border-l-transparent p-[1px]",
				orientation === "horizontal" &&
					"h-2.5 flex-col border-t border-t-transparent p-[1px]",
				className,
			)}
			{...props}
		>
			<ScrollAreaPrimitive.ScrollAreaThumb className="relative flex-1 rounded-full bg-surface-quaternary" />
		</ScrollAreaPrimitive.ScrollAreaScrollbar>
	);
};
