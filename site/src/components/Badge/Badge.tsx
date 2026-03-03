/**
 * Copied from shadcn/ui on 11/13/2024
 * @see {@link https://ui.shadcn.com/docs/components/badge}
 */
import { Slot } from "@radix-ui/react-slot";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "utils/cn";

const badgeVariants = cva(
	`
	inline-flex items-center gap-1 rounded-md border px-1.5 py-0.5 text-nowrap
	transition-colors [&_svg]:py-0.5
	[&_svg]:pointer-events-none
	`,
	{
		variants: {
			variant: {
				default:
					"border border-solid border-surface-secondary bg-surface-secondary text-content-secondary shadow hover:bg-surface-tertiary",
				warning:
					"border border-solid border-border-warning bg-surface-orange text-content-warning shadow",
				destructive:
					"border border-solid border-border-destructive bg-surface-red text-highlight-red shadow",
				green:
					"border border-solid border-border-green bg-surface-green text-highlight-green shadow",
				purple:
					"border border-solid border-border-purple bg-surface-purple text-highlight-purple shadow",
				magenta:
					"border border-solid border-border-magenta bg-surface-magenta text-highlight-magenta shadow",
				info: "border border-solid border-border-pending bg-surface-sky text-highlight-sky shadow",
			},
			size: {
				xs: "border-0 text-2xs font-normal h-4.5 [&_svg]:size-icon-xs rounded",
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
	const Comp = asChild ? Slot : "div";

	return (
		<Comp
			{...props}
			className={cn(badgeVariants({ variant, size, hover }), className)}
		/>
	);
};
