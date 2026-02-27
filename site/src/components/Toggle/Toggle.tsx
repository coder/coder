/**
 * Copied from shadc/ui on 02/27/2026
 * @see {@link https://ui.shadcn.com/docs/components/toggle}
 */
import * as TogglePrimitive from "@radix-ui/react-toggle";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "utils/cn";

export const toggleVariants = cva(
	`inline-flex items-center justify-center rounded-md border border-solid border-transparent
	text-sm font-medium transition-colors shrink-0 cursor-pointer
	focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-content-link
	disabled:pointer-events-none disabled:text-content-disabled
	[&>svg]:pointer-events-none [&>svg]:shrink-0`,
	{
		variants: {
			variant: {
				default: `
					bg-transparent text-content-secondary
					hover:text-content-primary
					data-[state=on]:bg-surface-invert-primary data-[state=on]:text-content-invert
					data-[state=on]:hover:bg-surface-invert-secondary
					`,
				outline: `
					border-border-default bg-transparent text-content-secondary
					hover:text-content-primary hover:bg-surface-secondary
					data-[state=on]:border-border-default
					data-[state=on]:bg-surface-secondary data-[state=on]:text-content-primary
					data-[state=on]:hover:bg-surface-secondary
					`,
			},
			size: {
				default: "h-9 px-3 [&_svg]:size-icon-sm",
				sm: "h-7 px-2 [&_svg]:size-icon-sm",
				lg: "h-10 px-3 [&_svg]:size-icon-lg",
			},
		},
		defaultVariants: {
			variant: "default",
			size: "default",
		},
	},
);

export type ToggleProps = React.ComponentPropsWithRef<
	typeof TogglePrimitive.Root
> &
	VariantProps<typeof toggleVariants>;

export const Toggle: React.FC<ToggleProps> = ({
	className,
	variant,
	size,
	...props
}) => {
	return (
		<TogglePrimitive.Root
			className={cn(toggleVariants({ variant, size }), className)}
			{...props}
		/>
	);
};
