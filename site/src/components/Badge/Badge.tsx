/**
 * Copied from shadcn/ui on 11/13/2024
 * @see {@link https://ui.shadcn.com/docs/components/badge}
 */
import { cva, type VariantProps } from "class-variance-authority";
import { Slot } from "radix-ui";
import { cn } from "#/utils/cn";

const badgeVariants = cva(
	`
	inline-flex items-center gap-1 rounded-md border px-1.5 py-0.5 text-nowrap
	transition-colors [&_svg]:py-0.5 border-solid
	[&_svg]:pointer-events-none
	`,
	{
		variants: {
			variant: {
				default:
					"border-surface-secondary bg-surface-secondary text-content-secondary shadow",
				warning:
					"border-border-warning bg-surface-orange text-content-warning shadow",
				destructive:
					"border-border-destructive bg-surface-red text-highlight-red shadow",
				green:
					"border-border-green bg-surface-green text-highlight-green shadow",
				purple:
					"border-border-purple bg-surface-purple text-highlight-purple shadow",
				magenta:
					"border-border-magenta bg-surface-magenta text-highlight-magenta shadow",
				info: "border-border-pending bg-surface-sky text-highlight-sky shadow",
			},
			size: {
				xs: "border-0 text-2xs font-normal h-[18px] [&_svg]:size-icon-xs rounded",
				sm: "text-2xs font-normal h-5.5 py-1 [&_svg]:size-icon-xs",
				md: "text-xs font-normal py-1 [&_svg]:size-icon-xs",
			},
			hover: {
				false: null,
				true: "no-underline focus:outline-none focus-visible:ring-2 focus-visible:ring-content-link",
			},
		},
		compoundVariants: [
			{
				hover: true,
				variant: "default",
				class: "hover:bg-surface-tertiary",
			},
			{
				hover: true,
				variant: "info",
				class: "hover:bg-surface-info/20",
			},
		],
		defaultVariants: {
			variant: "default",
			size: "md",
			hover: false,
		},
	},
);

type BadgeProps = React.ComponentPropsWithRef<"div"> &
	VariantProps<typeof badgeVariants> & {
		asChild?: boolean;
	};

export const Badge: React.FC<BadgeProps> = ({
	className,
	variant,
	size,
	hover,
	asChild = false,
	...props
}) => {
	const Comp = asChild ? Slot.Root : "div";

	return (
		<Comp
			{...props}
			className={cn(badgeVariants({ variant, size, hover }), className)}
		/>
	);
};
