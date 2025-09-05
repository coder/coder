import { Slot, Slottable } from "@radix-ui/react-slot";
import { cva, type VariantProps } from "class-variance-authority";
import { SquareArrowOutUpRightIcon } from "lucide-react";
import { forwardRef } from "react";
import { cn } from "utils/cn";

const linkVariants = cva(
	`relative inline-flex items-center no-underline font-medium text-content-link hover:cursor-pointer
	 hover:after:content-[''] hover:after:absolute hover:after:left-0 hover:after:w-full hover:after:h-px hover:after:bg-current hover:after:bottom-px
	 focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link
	 focus-visible:ring-offset-2 focus-visible:ring-offset-surface-primary focus-visible:rounded-sm
	 visited:text-content-link pl-0.5`, //pl-0.5 adjusts the underline spacing to align with the icon on the right.
	{
		variants: {
			size: {
				lg: "text-sm gap-0.5 [&_svg]:size-icon-sm [&_svg]:p-0.5 leading-6",
				sm: "text-xs gap-1 [&_svg]:size-icon-xs [&_svg]:p-px leading-5",
			},
		},
		defaultVariants: {
			size: "lg",
		},
	},
);

interface LinkProps
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
