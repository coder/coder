/**
 * Copied from shadc/ui on 03/05/2025
 * @see {@link https://ui.shadcn.com/docs/components/scroll-area}
 */
import { ScrollArea as ScrollAreaPrimitive } from "radix-ui";
import { useEffect, useRef } from "react";
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
	// viewport overflows horizontally but not vertically, so a plain mouse
	// wheel scrolls the block instead of the page (e.g. a wide "both" code
	// block or a "horizontal" row of tabs). React attaches `wheel` listeners
	// as passive, where preventDefault is a no-op, so attach a non-passive
	// listener directly on the viewport.
	useEffect(() => {
		const el = viewportRef.current;
		if (!el || orientation === "vertical") return;
		const handleWheel = (e: WheelEvent) => {
			// Only redirect a predominantly vertical gesture, and only when
			// there is horizontal overflow and no vertical overflow to scroll.
			if (Math.abs(e.deltaY) <= Math.abs(e.deltaX)) return;
			if (el.scrollWidth <= el.clientWidth) return;
			if (el.scrollHeight > el.clientHeight) return;
			// Let the page keep scrolling once the block reaches a horizontal
			// edge, so the wheel isn't trapped at the boundaries.
			const maxLeft = el.scrollWidth - el.clientWidth;
			if (e.deltaY > 0 && el.scrollLeft >= maxLeft) return;
			if (e.deltaY < 0 && el.scrollLeft <= 0) return;
			e.preventDefault();
			el.scrollBy({ left: e.deltaY, behavior: "smooth" });
		};
		el.addEventListener("wheel", handleWheel, { passive: false });
		return () => el.removeEventListener("wheel", handleWheel);
	}, [orientation]);

	return (
		<ScrollAreaPrimitive.Root
			className={cn("relative overflow-hidden", className)}
			{...props}
		>
			<ScrollAreaPrimitive.Viewport
				ref={viewportRef}
				tabIndex={viewportTabIndex}
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
			<ScrollAreaPrimitive.ScrollAreaThumb
				className={cn(
					// The visible thumb stays slim. `surface-invert-secondary`
					// keeps a >=3:1 contrast (WCAG 1.4.11 Non-text Contrast)
					// against the surfaces the scrollbar overlays in both light
					// and dark themes; `surface-quaternary` fell well below 3:1.
					"relative flex-1 rounded-full bg-surface-invert-secondary",
					// A transparent `::before` enlarges the pointer/drag target to
					// at least 24px (WCAG 2.5.8 Target Size (Minimum)) without
					// thickening the visible thumb. It extends inward from the
					// scroll area's outer edge so it is never clipped by the
					// viewport's overflow.
					"before:absolute before:content-['']",
					orientation === "vertical"
						? "before:right-0 before:top-1/2 before:h-full before:min-h-6 before:w-6 before:-translate-y-1/2"
						: "before:bottom-0 before:left-1/2 before:w-full before:min-w-6 before:h-6 before:-translate-x-1/2",
				)}
			/>
		</ScrollAreaPrimitive.ScrollAreaScrollbar>
	);
};
