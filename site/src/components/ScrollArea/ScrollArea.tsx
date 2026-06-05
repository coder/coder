/**
 * Copied from shadc/ui on 03/05/2025
 * @see {@link https://ui.shadcn.com/docs/components/scroll-area}
 */
import { ScrollArea as ScrollAreaPrimitive } from "radix-ui";
import { useCallback, useRef } from "react";
import { cn } from "#/utils/cn";

interface ScrollAreaProps
	extends React.ComponentPropsWithRef<typeof ScrollAreaPrimitive.Root> {
	scrollBarClassName?: string;
	/**
	 * Class for the horizontal scrollbar. Sets its thickness
	 * independently of the vertical bar. For orientation "horizontal"
	 * it falls back to scrollBarClassName when omitted.
	 */
	horizontalScrollBarClassName?: string;
	viewportClassName?: string;
	viewportTabIndex?: number;
	/** Which scrollbar(s) to show. Defaults to "vertical". */
	orientation?: "vertical" | "horizontal" | "both";
}

export const ScrollArea: React.FC<ScrollAreaProps> = ({
	className,
	scrollBarClassName,
	horizontalScrollBarClassName,
	viewportClassName,
	viewportTabIndex,
	orientation = "vertical",
	children,
	...props
}) => {
	const viewportRef = useRef<HTMLDivElement>(null);

	// Redirect a vertical wheel gesture into horizontal scroll when the
	// viewport overflows horizontally but not vertically. Without this a
	// plain mouse wheel scrolls the page instead of the container (e.g. a
	// wide "both" code block or a "horizontal" row of tabs).
	const handleWheel = useCallback(
		(e: React.WheelEvent<HTMLDivElement>) => {
			if (orientation === "vertical") return;
			const el = viewportRef.current;
			if (!el) return;
			// Only redirect a predominantly vertical gesture, and only when
			// there is horizontal overflow and no vertical overflow to scroll.
			if (Math.abs(e.deltaY) <= Math.abs(e.deltaX)) return;
			if (el.scrollWidth <= el.clientWidth) return;
			if (el.scrollHeight > el.clientHeight) return;
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
				tabIndex={viewportTabIndex}
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
					className={cn(
						"z-10",
						// scrollBarClassName sizes the vertical bar, so only fall
						// back to it for a horizontal-only area. For "both", an
						// unset horizontal class keeps the built-in thickness.
						orientation === "both"
							? horizontalScrollBarClassName
							: (horizontalScrollBarClassName ?? scrollBarClassName),
					)}
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
