/**
 * Copied from shadc/ui on 03/05/2025
 * @see {@link https://ui.shadcn.com/docs/components/scroll-area}
 */
import { ScrollArea as ScrollAreaPrimitive } from "radix-ui";
import { cn } from "#/utils/cn";

interface ScrollAreaProps
	extends React.ComponentPropsWithRef<typeof ScrollAreaPrimitive.Root> {
	scrollBarClassName?: string;
	horizontalScrollBarClassName?: string;
	/** Extra thumb classes; also reaches the thumb's `::before` hit-target. */
	scrollThumbClassName?: string;
	viewportClassName?: string;
	viewportTabIndex?: number;
	/** Which scrollbar(s) to show. Defaults to "vertical". */
	orientation?: "vertical" | "horizontal" | "both";
}

export const ScrollArea: React.FC<ScrollAreaProps> = ({
	className,
	scrollBarClassName,
	horizontalScrollBarClassName,
	scrollThumbClassName,
	viewportClassName,
	viewportTabIndex,
	orientation = "vertical",
	children,
	...props
}) => {
	return (
		<ScrollAreaPrimitive.Root
			className={cn("relative overflow-hidden", className)}
			{...props}
		>
			<ScrollAreaPrimitive.Viewport
				tabIndex={viewportTabIndex}
				className={cn("h-full w-full rounded-[inherit]", viewportClassName)}
			>
				{children}
			</ScrollAreaPrimitive.Viewport>
			{(orientation === "vertical" || orientation === "both") && (
				<ScrollBar
					orientation="vertical"
					className={cn("z-10", scrollBarClassName)}
					thumbClassName={scrollThumbClassName}
				/>
			)}
			{(orientation === "horizontal" || orientation === "both") && (
				<ScrollBar
					orientation="horizontal"
					className={cn(
						"z-10",
						orientation === "both"
							? horizontalScrollBarClassName
							: (horizontalScrollBarClassName ?? scrollBarClassName),
					)}
					thumbClassName={scrollThumbClassName}
				/>
			)}
			<ScrollAreaPrimitive.Corner />
		</ScrollAreaPrimitive.Root>
	);
};

export const ScrollBar: React.FC<
	React.ComponentPropsWithRef<
		typeof ScrollAreaPrimitive.ScrollAreaScrollbar
	> & {
		thumbClassName?: string;
	}
> = ({ className, orientation = "vertical", thumbClassName, ...props }) => {
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
					"relative flex-1 rounded-full bg-surface-invert-secondary",
					"before:absolute before:content-['']",
					orientation === "vertical"
						? "before:right-0 before:top-1/2 before:h-full before:min-h-6 before:w-6 before:-translate-y-1/2"
						: "before:bottom-0 before:left-1/2 before:w-full before:min-w-6 before:h-6 before:-translate-x-1/2",
					thumbClassName,
				)}
			/>
		</ScrollAreaPrimitive.ScrollAreaScrollbar>
	);
};
