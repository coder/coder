/**
 * Copied from shadc/ui on 02/05/2025
 * @see {@link https://ui.shadcn.com/docs/components/tooltip}
 */
import * as TooltipPrimitive from "@radix-ui/react-tooltip";
import { cloneElement, isValidElement, type ReactElement } from "react";
import { cn } from "utils/cn";

export const TooltipProvider = TooltipPrimitive.Provider;

export type TooltipProps = TooltipPrimitive.TooltipProps;

export const Tooltip = TooltipPrimitive.Root;

// When asChild is used with non-focusable elements (div, span, Pill,
// Badge, SVG icons), they need tabIndex to be reachable via keyboard.
// Radix's TooltipTrigger uses Primitive.button internally which is
// natively focusable, but Slot (used with asChild) doesn't add tabIndex
// to non-focusable elements. We inject tabIndex={0} unless the child
// already specifies one.
export const TooltipTrigger: React.FC<
	React.ComponentPropsWithRef<typeof TooltipPrimitive.Trigger>
> = ({ children, asChild, ref, ...props }) => {
	if (asChild && isValidElement(children)) {
		const childProps = children.props as Record<string, unknown>;
		if (childProps.tabIndex === undefined) {
			return (
				<TooltipPrimitive.Trigger asChild ref={ref} {...props}>
					{cloneElement(children as ReactElement<{ tabIndex?: number }>, {
						tabIndex: 0,
					})}
				</TooltipPrimitive.Trigger>
			);
		}
	}

	return (
		<TooltipPrimitive.Trigger asChild={asChild} ref={ref} {...props}>
			{children}
		</TooltipPrimitive.Trigger>
	);
};

export const TooltipArrow = TooltipPrimitive.Arrow;

export type TooltipContentProps = React.ComponentPropsWithRef<
	typeof TooltipPrimitive.Content
> & {
	disablePortal?: boolean;
};

export const TooltipContent: React.FC<TooltipContentProps> = ({
	className,
	sideOffset = 4,
	disablePortal,
	...props
}) => {
	const content = (
		<TooltipPrimitive.Content
			sideOffset={sideOffset}
			className={cn(
				"z-50 overflow-hidden rounded-md bg-surface-primary px-3 py-2 text-xs font-medium text-content-secondary",
				"border border-solid border-border animate-in fade-in-0 zoom-in-95",
				"data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=closed]:zoom-out-95",
				"data-[side=bottom]:slide-in-from-top-2 data-[side=left]:slide-in-from-right-2",
				"data-[side=right]:slide-in-from-left-2 data-[side=top]:slide-in-from-bottom-2",
				className,
			)}
			{...props}
		/>
	);

	return disablePortal ? (
		content
	) : (
		<TooltipPrimitive.Portal>{content}</TooltipPrimitive.Portal>
	);
};
