/**
 * Copied from shadc/ui on 11/13/2024
 * @see {@link https://ui.shadcn.com/docs/components/badge}
 */
import { Slot } from "@radix-ui/react-slot";
import { cva, type VariantProps } from "class-variance-authority";
import { forwardRef } from "react";
import { cn } from "utils/cn";

const badgeVariants = cva(
	`
	inline-flex items-center rounded-md border px-2 py-1 text-nowrap
	transition-colors
	[&_svg]:pointer-events-none [&_svg]:pr-0.5 [&_svg]:py-0.5 [&_svg]:mr-0.5
	`,
	{
		variants: {
			variant: {
				default:
					"border-transparent bg-surface-secondary text-content-secondary shadow",
				warning:
					"border border-solid border-border-warning bg-surface-orange text-content-warning shadow",
				destructive:
					"border border-solid border-border-destructive bg-surface-red text-highlight-red shadow",
				green:
					"border border-solid border-border-green bg-surface-green text-highlight-green shadow",
				info: "border border-solid border-border-sky bg-surface-sky text-highlight-sky shadow",
			},
			size: {
				xs: "text-2xs font-regular h-5 [&_svg]:hidden rounded px-1.5",
				sm: "text-2xs font-regular h-5.5 [&_svg]:size-icon-xs",
				md: "text-xs font-medium [&_svg]:size-icon-sm",
			},
			border: {
				none: "border-transparent",
				solid: "border border-solid",
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
			border: "none",
			hover: false,
		},
	},
);

interface BadgeProps
	extends React.HTMLAttributes<HTMLDivElement>,
		VariantProps<typeof badgeVariants> {
	asChild?: boolean;
}

export const Badge = forwardRef<HTMLDivElement, BadgeProps>(
	(
		{ className, variant, size, border, hover, asChild = false, ...props },
		ref,
	) => {
		const Comp = asChild ? Slot : "div";

		return (
			<Comp
				{...props}
				ref={ref}
				className={cn(
					badgeVariants({ variant, size, border, hover }),
					className,
				)}
			/>
		);
	},
);
