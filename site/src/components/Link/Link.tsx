import { type VariantProps, cva } from "class-variance-authority";
import { SquareArrowOutUpRight } from "lucide-react";
import { forwardRef } from "react";
import { cn } from "utils/cn";

export const linkVariants = cva(
	`relative inline-flex items-center no-underline font-medium text-content-link
	 after:hover:content-[''] after:hover:absolute after:hover:w-full after:hover:h-[1px] after:hover:bg-current after:hover:bottom-px
	 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-content-link
	 focus-visible:ring-offset-2 focus-visible:ring-offset-surface-primary focus-visible:rounded-sm
	 visited:text-content-link`,
	{
		variants: {
			size: {
				lg: "text-sm mb-px",
				sm: "text-xs [&_svg]:size-icon-xs [&_svg]:p-px",
			},
		},
		defaultVariants: {
			size: "lg",
		},
	},
);

export interface LinkProps
	extends React.AnchorHTMLAttributes<HTMLAnchorElement>,
		VariantProps<typeof linkVariants> {
	text: string;
}

export const Link = forwardRef<HTMLAnchorElement, LinkProps>(
	({ className, text, size, ...props }, ref) => {
		return (
			<a className={cn(linkVariants({ size }), className)} ref={ref} {...props}>
				{text}&nbsp;
				<SquareArrowOutUpRight size={14} aria-hidden="true" />
			</a>
		);
	},
);
