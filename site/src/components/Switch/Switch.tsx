import { Switch as SwitchPrimitives } from "@base-ui/react/switch";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "cn";

const switchVariants = cva(
	`peer group/switch inline-flex shrink-0 cursor-pointer items-center overflow-hidden rounded-full border transition-colors
	focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link
	focus-visible:ring-offset-2 focus-visible:ring-offset-surface-primary
	disabled:cursor-not-allowed aria-disabled:cursor-not-allowed
	data-checked:border-0 data-checked:p-px
	data-unchecked:border-border-control
	data-unchecked:hover:enabled:not-aria-disabled:border-border-control-hover
	data-unchecked:disabled:border-border data-unchecked:aria-disabled:border-border`,
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
	data-checked:shadow-[0_0_0_2.5rem] data-checked:shadow-surface-invert-primary
	data-checked:group-hover/switch:group-enabled/switch:group-not-aria-disabled/switch:shadow-surface-invert-secondary
	data-checked:data-disabled:shadow-surface-tertiary
	data-checked:group-aria-disabled/switch:shadow-surface-tertiary
	data-unchecked:translate-x-px data-unchecked:bg-border-control
	data-unchecked:group-hover/switch:group-enabled/switch:group-not-aria-disabled/switch:bg-border-control-hover
	data-unchecked:data-disabled:bg-border
	data-unchecked:group-aria-disabled/switch:bg-border`,
	{
		variants: {
			size: {
				default: "size-4 data-checked:translate-x-4.25",
				sm: "size-3 data-checked:translate-x-3.25",
			},
		},
		defaultVariants: {
			size: "default",
		},
	},
);

type SwitchProps = React.ComponentProps<typeof SwitchPrimitives.Root> &
	Pick<React.ComponentProps<"button">, "type"> &
	VariantProps<typeof switchVariants>;

export const Switch: React.FC<SwitchProps> = ({
	className,
	size,
	type = "button",
	...props
}) => (
	<SwitchPrimitives.Root
		nativeButton
		render={(buttonProps) => <button {...buttonProps} type={type} />}
		className={(state) =>
			cn(
				switchVariants({ size }),
				typeof className === "function" ? className(state) : className,
			)
		}
		{...props}
	>
		<SwitchPrimitives.Thumb className={cn(thumbVariants({ size }))} />
	</SwitchPrimitives.Root>
);
