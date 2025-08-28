/**
 * Copied from shadcn/ui and modified on 8/27/2025
 * @see {@link https://ui.shadcn.com/docs/components/hover-card}
 */

import * as HoverCardPrimitive from "@radix-ui/react-hover-card";
import {
	type ComponentPropsWithoutRef,
	type ElementRef,
	forwardRef,
} from "react";
import { cn } from "utils/cn";

const HoverCard = HoverCardPrimitive.Root;

const HoverCardTrigger = HoverCardPrimitive.Trigger;

const HoverCardContent = forwardRef<
	ElementRef<typeof HoverCardPrimitive.Content>,
	ComponentPropsWithoutRef<typeof HoverCardPrimitive.Content>
>(({ className, align = "center", sideOffset = 0, ...props }, ref) => (
	<HoverCardPrimitive.Content
		ref={ref}
		align={align}
		sideOffset={sideOffset}
		className={cn(
			"z-50 w-64 rounded-md border border-solid border-surface-quaternary bg-surface-secondary p-4 text-popover-foreground shadow-md outline-none data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95 data-[side=bottom]:slide-in-from-top-2 data-[side=left]:slide-in-from-right-2 data-[side=right]:slide-in-from-left-2 data-[side=top]:slide-in-from-bottom-2 origin-[--radix-hover-card-content-transform-origin]",
			className,
		)}
		{...props}
	/>
));
HoverCardContent.displayName = HoverCardPrimitive.Content.displayName;

export { HoverCard, HoverCardTrigger, HoverCardContent };
