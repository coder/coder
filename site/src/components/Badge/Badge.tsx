/**
 * Copied from shadc/ui on 11/13/2024
 * @see {@link https://ui.shadcn.com/docs/components/badge}
 */
import { type VariantProps, cva } from "class-variance-authority";
import type { FC } from "react";

import { cn } from "utils/cn";

export const badgeVariants = cva(
	"inline-flex items-center rounded-md border px-2.5 py-1 text-xs font-semibold transition-colors focus:outline-none focus:ring-2 focus:ring-ring focus:ring-offset-2",
	{
		variants: {
			variant: {
				default:
					"border-transparent bg-surface-secondary text-content-secondary shadow hover:bg-surface-tertiary",
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

export const Badge: FC<BadgeProps> = ({ className, variant, ...props }) => {
	return (
		<div className={cn(badgeVariants({ variant }), className)} {...props} />
	);
};
