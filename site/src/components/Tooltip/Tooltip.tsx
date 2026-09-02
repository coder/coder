/**
 * Copied from shadc/ui on 02/05/2025
 * @see {@link https://ui.shadcn.com/docs/components/tooltip}
 */

import { cn } from "cn";
import { Tooltip as TooltipPrimitive } from "radix-ui";

export const TooltipProvider = TooltipPrimitive.Provider;

/**
 * Shared open delay (ms) for tooltips. Used by the app-wide provider and by
 * self-contained tooltips such as InfoTooltip so hover timing stays consistent.
 */
export const TOOLTIP_DELAY_DURATION = 100;

export const Tooltip = TooltipPrimitive.Root;

export const TooltipTrigger = TooltipPrimitive.Trigger;

export const TooltipArrow = TooltipPrimitive.Arrow;

type TooltipContentProps = React.ComponentPropsWithRef<
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

/**
 * Presentational heading for tooltip content. Kept generic so both InfoTooltip
 * and raw Tooltip call sites share one title style.
 */
export const TooltipTitle: React.FC<
	React.HTMLAttributes<HTMLParagraphElement>
> = ({ className, ...props }) => (
	<p
		className={cn("m-0 mb-1 font-semibold text-content-primary", className)}
		{...props}
	/>
);

/**
 * Presentational body text for tooltip content. Embedded links get top spacing
 * so they read as a separate action beneath the message.
 */
export const TooltipMessage: React.FC<
	React.HTMLAttributes<HTMLParagraphElement>
> = ({ className, ...props }) => (
	<p
		className={cn("m-0 text-content-secondary [&_a]:mt-2", className)}
		{...props}
	/>
);
