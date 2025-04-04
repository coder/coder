/**
 * Copied from shadc/ui on 04/03/2025
 * @see {@link https://ui.shadcn.com/docs/components/checkbox}
 */
import * as CheckboxPrimitive from "@radix-ui/react-checkbox";
import { Check, Minus } from "lucide-react";
import * as React from "react";

import { cn } from "utils/cn";

export const Checkbox = React.forwardRef<
	React.ElementRef<typeof CheckboxPrimitive.Root>,
	React.ComponentPropsWithoutRef<typeof CheckboxPrimitive.Root>
>(({ className, ...props }, ref) => (
	<CheckboxPrimitive.Root
		ref={ref}
		className={cn(
			`peer h-6 w-6 shrink-0 rounded-sm border border-border border-solid
    	focus-visible:outline-none focus-visible:ring-2
    	focus-visible:ring-content-link focus-visible:ring-offset-4 focus-visible:ring-offset-surface-primary
    	disabled:cursor-not-allowed disabled:bg-surface-primary disabled:data-[state=checked]:bg-surface-tertiary
    	data-[state=unchecked]:bg-surface-primary
    	data-[state=checked]:bg-surface-invert-primary data-[state=checked]:text-content-invert
    	hover:border-border-hover hover:data-[state=checked]:bg-surface-invert-secondary`,
			className,
		)}
		{...props}
	>
		<CheckboxPrimitive.Indicator
			className={cn("flex items-center justify-center text-current relative")}
		>
			<div className="flex">
				{(props.checked === true || props.defaultChecked === true) && (
					<Check className="w-5 h-5" strokeWidth={2.5} />
				)}
				{props.checked === "indeterminate" && (
					<Minus className="w-5 h-5" strokeWidth={2.5} />
				)}
			</div>
		</CheckboxPrimitive.Indicator>
	</CheckboxPrimitive.Root>
));
