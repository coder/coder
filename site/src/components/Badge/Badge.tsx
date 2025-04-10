/**
 * Copied from shadc/ui on 11/13/2024
 * @see {@link https://ui.shadcn.com/docs/components/badge}
 */
import { type VariantProps, cva } from "class-variance-authority";
import  { forwardRef } from "react";
import { Slot } from "@radix-ui/react-slot";
import { cn } from "utils/cn";

export const badgeVariants = cva(
	`inline-flex items-center rounded-md border px-2 py-1 transition-colors
	focus:outline-none focus:ring-2 focus:ring-ring focus:ring-offset-2
	[&_svg]:pointer-events-none [&_svg]:pr-0.5 [&_svg]:py-0.5 [&_svg]:mr-0.5`,
	{
		variants: {
			variant: {
				default:
					"border-transparent bg-surface-secondary text-content-secondary shadow",
				warning:
					"border-transparent bg-surface-orange text-content-warning shadow",
			},
			size: {
				sm: "text-2xs font-regular h-5.5 [&_svg]:size-icon-xs",
				md: "text-xs font-medium [&_svg]:size-icon-sm",
			},
		},
		defaultVariants: {
			variant: "default",
			size: "md",
		},
	},
);

export interface BadgeProps
	extends React.HTMLAttributes<HTMLDivElement>,
		VariantProps<typeof badgeVariants> {
			asChild?: boolean;
		}

export const Badge = forwardRef<HTMLDivElement, BadgeProps>(
	({ className, variant, size, asChild = false, ...props }, ref) => {
		const Comp = asChild ? Slot : "div"

		return (
			<Comp
				{...props}
				ref={ref}
				className={cn(badgeVariants({ variant, size }), className)}
			/>
		);
	},
);
