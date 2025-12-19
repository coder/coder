/**
 * Copied from shadcn/ui and modified on 12/13/2024
 * @see {@link https://ui.shadcn.com/docs/components/popover}
 */
import * as PopoverPrimitive from "@radix-ui/react-popover";
import {
	type ComponentPropsWithoutRef,
	type ElementRef,
	forwardRef,
} from "react";
import { cn } from "utils/cn";

export type PopoverContentProps = PopoverPrimitive.PopoverContentProps;

export type PopoverTriggerProps = PopoverPrimitive.PopoverTriggerProps;

export const Popover = PopoverPrimitive.Root;

export const PopoverTrigger = PopoverPrimitive.Trigger;

export const PopoverClose = PopoverPrimitive.PopoverClose;

export const PopoverContent = forwardRef<
	ElementRef<typeof PopoverPrimitive.Content>,
	ComponentPropsWithoutRef<typeof PopoverPrimitive.Content>
>(({ className, align = "center", sideOffset = 4, ...props }, ref) => (
	<PopoverPrimitive.Portal>
		<PopoverPrimitive.Content
			ref={ref}
			align={align}
			sideOffset={sideOffset}
			collisionPadding={16}
			className={cn(
				// z-[1400] is required to appear above MUI dialogs (z-index: 1300) during
				// migration. Can be reduced to z-50 once MUI is fully removed.
				`z-[1400] w-72 rounded-md border border-solid bg-surface-primary
			text-content-primary shadow-md outline-none
			data-[state=open]:animate-in data-[state=closed]:animate-out
			data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0
			data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95
			data-[side=bottom]:slide-in-from-top-2 data-[side=left]:slide-in-from-right-2
			data-[side=right]:slide-in-from-left-2 data-[side=top]:slide-in-from-bottom-2`,
				className,
			)}
			{...props}
		/>
	</PopoverPrimitive.Portal>
));
