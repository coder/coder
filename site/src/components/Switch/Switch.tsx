/**
 * Copied from shadc/ui on 11/13/2024
 * @see {@link https://ui.shadcn.com/docs/components/switch}
 */
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "cn";
import { Switch as SwitchPrimitives } from "radix-ui";

const switchVariants = cva(
	`peer group/switch inline-flex shrink-0 cursor-pointer items-center overflow-hidden rounded-full border transition-colors
	focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link
	focus-visible:ring-offset-2 focus-visible:ring-offset-surface-primary
	disabled:cursor-not-allowed aria-disabled:cursor-not-allowed
	data-[state=checked]:border-0 data-[state=checked]:p-px
	data-[state=unchecked]:border-border-control
	data-[state=unchecked]:hover:enabled:not-aria-disabled:border-border-control-hover
	data-[state=unchecked]:disabled:border-border data-[state=unchecked]:aria-disabled:border-border`,
	{
		variants: {
			size: {
				default: "h-5 w-9",
				sm: "h-4 w-7",
			},
		},
		defaultVariants: {
			size: "default",
		},
	},
);

const thumbVariants = cva(
	`pointer-events-none block rounded-full ring-0 transition
	data-[state=checked]:shadow-[0_0_0_2.5rem] data-[state=checked]:shadow-surface-invert-primary
	data-[state=checked]:group-hover/switch:group-enabled/switch:group-not-aria-disabled/switch:shadow-surface-invert-secondary
	data-[state=checked]:data-disabled:shadow-surface-tertiary
	data-[state=checked]:group-aria-disabled/switch:shadow-surface-tertiary
	data-[state=unchecked]:translate-x-px data-[state=unchecked]:bg-border-control
	data-[state=unchecked]:group-hover/switch:group-enabled/switch:group-not-aria-disabled/switch:bg-border-control-hover
	data-[state=unchecked]:data-disabled:bg-border
	data-[state=unchecked]:group-aria-disabled/switch:bg-border`,
	{
		variants: {
			size: {
				default: "size-4 data-[state=checked]:translate-x-4.25",
				sm: "size-3 data-[state=checked]:translate-x-3.25",
			},
		},
		defaultVariants: {
			size: "default",
		},
	},
);

type SwitchProps = React.ComponentProps<typeof SwitchPrimitives.Root> &
	VariantProps<typeof switchVariants>;

export const Switch: React.FC<SwitchProps> = ({
	className,
	size,
	...props
}) => (
	<SwitchPrimitives.Root
		className={cn(switchVariants({ size }), className)}
		{...props}
	>
		<SwitchPrimitives.Thumb className={cn(thumbVariants({ size }))} />
	</SwitchPrimitives.Root>
);
