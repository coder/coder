import { type VariantProps, cva } from "class-variance-authority";
import type * as React from "react";

import { cn } from "utils/cn";

const badgeVariants = cva(
	"inline-flex items-center rounded-md border px-2.5 py-0.5 text-xs font-semibold transition-colors focus:outline-none focus:ring-2 focus:ring-ring focus:ring-offset-2",
	{
		variants: {
			variant: {
				default:
					"border-transparent bg-surface-secondary text-content-secondary shadow hover:bg-surface-tertiary",
				secondary:
					"border-transparent bg-surface-secondary text-content-secondary hover:bg-surface-tertiary",
				destructive:
					"border-transparent bg-surface-error text-content-danger shadow hover:bg-surface-error/80",
				outline: "text-content-primary",
			},
		},
		defaultVariants: {
			variant: "default",
		},
	},
);

export interface BadgeProps
	extends React.HTMLAttributes<HTMLDivElement>,
		VariantProps<typeof badgeVariants> {}

function Badge({ className, variant, ...props }: BadgeProps) {
	return (
		<div className={cn(badgeVariants({ variant }), className)} {...props} />
	);
}

export { Badge, badgeVariants };
