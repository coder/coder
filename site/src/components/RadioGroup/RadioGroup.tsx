/**
 * Copied from shadc/ui on 04/04/2025
 * @see {@link https://ui.shadcn.com/docs/components/radio-group}
 */
import * as RadioGroupPrimitive from "@radix-ui/react-radio-group";
import { Circle } from "lucide-react";
import * as React from "react";

import { cn } from "utils/cn";

export const RadioGroup = React.forwardRef<
	React.ElementRef<typeof RadioGroupPrimitive.Root>,
	React.ComponentPropsWithoutRef<typeof RadioGroupPrimitive.Root>
>(({ className, ...props }, ref) => {
	return (
		<RadioGroupPrimitive.Root
			className={cn("grid gap-2", className)}
			{...props}
			ref={ref}
		/>
	);
});
RadioGroup.displayName = RadioGroupPrimitive.Root.displayName;

export const RadioGroupItem = React.forwardRef<
	React.ElementRef<typeof RadioGroupPrimitive.Item>,
	React.ComponentPropsWithoutRef<typeof RadioGroupPrimitive.Item>
>(({ className, ...props }, ref) => {
	return (
		<RadioGroupPrimitive.Item
			ref={ref}
			className={cn(
				`aspect-square h-4 w-4 rounded-full border border-solid border-border text-content-primary bg-surface-primary
			focus:outline-none focus-visible:ring-2 focus-visible:ring-content-link
			focus-visible:ring-offset-4 focus-visible:ring-offset-surface-primary
			disabled:cursor-not-allowed disabled:opacity-25 disabled:border-surface-invert-primary
			hover:border-border-hover`,
				className,
			)}
			{...props}
		>
			<RadioGroupPrimitive.Indicator className="flex items-center justify-center">
				<Circle className="absolute h-2.5 w-2.5 fill-surface-invert-primary" />
			</RadioGroupPrimitive.Indicator>
		</RadioGroupPrimitive.Item>
	);
});
