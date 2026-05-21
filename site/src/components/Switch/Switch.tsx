/**
 * Copied from shadc/ui on 11/13/2024
 * @see {@link https://ui.shadcn.com/docs/components/switch}
 */
import { cva, type VariantProps } from "class-variance-authority";
import { Switch as SwitchPrimitives } from "radix-ui";
import { cn } from "#/utils/cn";

const switchVariants = cva(
	`peer inline-flex shrink-0 cursor-pointer items-center rounded-full shadow-sm transition-colors
	border-2 border-transparent
	focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-content-link
	focus-visible:ring-offset-2 focus-visible:ring-offset-surface-primary
	disabled:cursor-not-allowed
	data-[state=checked]:disabled:bg-surface-tertiary data-[state=unchecked]:disabled:bg-surface-tertiary
	data-[state=checked]:hover:bg-surface-invert-secondary data-[state=unchecked]:hover:bg-surface-tertiary
	data-[state=checked]:bg-surface-invert-primary data-[state=unchecked]:bg-surface-quaternary`,
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
	"pointer-events-none block rounded-full bg-surface-primary shadow-lg ring-0 transition-transform",
	{
		variants: {
			size: {
				default:
					"h-4 w-4 data-[state=checked]:translate-x-2.5 data-[state=unchecked]:-translate-x-1.5",
				sm: "h-3 w-3 data-[state=checked]:translate-x-1.5 data-[state=unchecked]:-translate-x-1.5",
			},
		},
		defaultVariants: {
			size: "default",
		},
	},
);

type SwitchProps = React.ComponentPropsWithRef<typeof SwitchPrimitives.Root> &
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
