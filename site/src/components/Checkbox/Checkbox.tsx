/**
 * Copied from shadc/ui on 04/03/2025
 * @see {@link https://ui.shadcn.com/docs/components/checkbox}
 */
import { Check, Minus } from "lucide-react";
import { Checkbox as CheckboxPrimitive } from "radix-ui";
import { cn } from "#/utils/cn";

/**
 * To allow for an indeterminate state the checkbox must be controlled, otherwise the checked prop would remain undefined
 */
export const Checkbox: React.FC<
	React.ComponentPropsWithRef<typeof CheckboxPrimitive.Root>
> = ({ className, ...props }) => {
	return (
		<CheckboxPrimitive.Root
			className={cn(
				"peer size-[18px] shrink-0 rounded-xs border border-border border-solid m-1",
				"focus-visible:outline-none focus-visible:ring-2 relative",
				"focus-visible:ring-content-link focus-visible:ring-offset-[3px] focus-visible:ring-offset-surface-primary",
				"disabled:cursor-not-allowed",
				"disabled:bg-surface-primary",
				"disabled:data-[state=checked]:bg-surface-tertiary",
				"disabled:data-[state=checked]:border-surface-tertiary",
				"data-[state=unchecked]:bg-surface-primary",
				"data-[state=checked]:bg-surface-invert-primary",
				"data-[state=checked]:text-content-invert",
				"data-[state=checked]:border-surface-invert-primary",
				"data-[state=indeterminate]:bg-surface-invert-primary",
				"data-[state=indeterminate]:text-content-invert",
				"data-[state=indeterminate]:border-surface-invert-primary",
				"hover:enabled:border-border-hover",
				"hover:data-[state=checked]:bg-surface-invert-secondary",
				"hover:data-[state=checked]:border-surface-invert-secondary",
				"hover:data-[state=indeterminate]:bg-surface-invert-secondary",
				"hover:data-[state=indeterminate]:border-surface-invert-secondary",
				className,
			)}
			{...props}
		>
			<CheckboxPrimitive.Indicator
				className={cn(
					"absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 size-4",
				)}
			>
				{(props.checked === true || props.defaultChecked === true) && (
					<Check className="size-4" strokeWidth={2.5} />
				)}
				{props.checked === "indeterminate" && (
					<Minus className="size-4" strokeWidth={2.5} />
				)}
			</CheckboxPrimitive.Indicator>
		</CheckboxPrimitive.Root>
	);
};
