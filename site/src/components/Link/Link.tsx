import { Slot, Slottable } from "@radix-ui/react-slot";
import { type VariantProps, cva } from "class-variance-authority";
import { SquareArrowOutUpRightIcon } from "lucide-react";
import { forwardRef } from "react";
import { cn } from "utils/cn";

export const linkVariants = cva(
	`relative inline-flex items-center no-underline font-medium text-content-link hover:cursor-pointer
	 after:hover:content-[''] after:hover:absolute after:hover:left-0 after:hover:w-full after:hover:h-[1px] after:hover:bg-current after:hover:bottom-px
	 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-content-link
	 focus-visible:ring-offset-2 focus-visible:ring-offset-surface-primary focus-visible:rounded-sm
	 visited:text-content-link pl-[2px]`, //pl-[2px] adjusts the underline spacing to align with the icon on the right.
	{
		variants: {
			size: {
				lg: "text-sm gap-[2px] [&_svg]:size-icon-sm [&_svg]:p-[2px] leading-6",
				sm: "text-xs gap-1 [&_svg]:size-icon-xs [&_svg]:p-[1px] leading-[18px]",
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
	asChild?: boolean;
}

export const Link = forwardRef<HTMLAnchorElement, LinkProps>(
	({ className, children, size, asChild, ...props }, ref) => {
		const Comp = asChild ? Slot : "a";
		return (
			<Comp
				className={cn(linkVariants({ size }), className)}
				ref={ref}
				{...props}
			>
				<Slottable>{children}</Slottable>
				<SquareArrowOutUpRightIcon aria-hidden="true" />
			</Comp>
		);
	},
);
