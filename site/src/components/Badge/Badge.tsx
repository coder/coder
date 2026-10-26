/**
 * Copied from shadcn/ui on 11/13/2024
 * @see {@link https://ui.shadcn.com/docs/components/badge}
 */
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "cn";
import { Slot } from "radix-ui";

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
					"border-transparent bg-surface-secondary text-content-secondary",
				outline:
					"border-border bg-transparent text-content-primary shadow-none",
				dashed:
					"border-dashed border-border bg-surface-secondary text-content-secondary shadow-none",
				warning:
					"border-highlight-orange bg-surface-orange text-highlight-orange",
				destructive:
					"border-border-destructive bg-surface-red text-highlight-red",
				green: "border-border-green bg-surface-green text-highlight-green",
				purple: "border-border-purple bg-surface-purple text-highlight-purple",
				magenta:
					"border-border-magenta bg-surface-magenta text-highlight-magenta",
				info: "border-border-pending bg-surface-sky text-highlight-sky",
			},
			size: {
				sm: "h-[18px] border-0 rounded-sm text-2xs font-normal",
				md: "h-[26px] py-1 text-xs font-normal",
			},
			svgSize: {
				xs: "[&_svg]:size-icon-xs",
				sm: "[&_svg]:size-icon-sm",
				lg: "[&_svg]:size-icon-lg",
			},
			hover: {
				false: null,
				true: "no-underline focus:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link",
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
				variant: "outline",
				class: "hover:bg-surface-secondary",
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
			svgSize: "xs",
			hover: false,
		},
	},
);

export type BadgeProps = React.ComponentProps<"div"> &
	VariantProps<typeof badgeVariants> & {
		asChild?: boolean;
	};

export const Badge: React.FC<BadgeProps> = ({
	className,
	variant,
	size,
	svgSize = "xs",
	hover,
	asChild = false,
	...props
}) => {
	const Comp = asChild ? Slot.Root : "div";

	return (
		<Comp
			{...props}
			className={cn(
				badgeVariants({ variant, size, svgSize, hover }),
				className,
			)}
		/>
	);
};

export const BadgeGroup: React.FC<React.PropsWithChildren> = ({ children }) => {
	return (
		<div className="flex flex-row items-center gap-2 mb-4">{children}</div>
	);
};
