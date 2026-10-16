import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "cn";
import { SquareArrowOutUpRightIcon } from "lucide-react";
import { Slot } from "radix-ui";

// py-1 -my-1 meets the WCAG 2.5.8 24px target without shifting layout, and the
// hover underline (bottom-1.25) relies on py-1. Override with pl-0, not p-0;
// subtract 4px from added vertical margin, or set my-0 when replacing padding.
// w-fit keeps flex and grid parents from stretching the link past its text.
const linkVariants = cva(
	`relative inline-flex items-center no-underline font-medium text-content-link hover:cursor-pointer
	 w-fit py-1 -my-1
	 hover:after:content-[''] hover:after:absolute hover:after:left-0 hover:after:w-full hover:after:h-px hover:after:bg-current hover:after:bottom-1.25
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

export type LinkProps = React.ComponentProps<"a"> &
	VariantProps<typeof linkVariants> & {
		asChild?: boolean;
		showExternalIcon?: boolean;
	};

export const Link: React.FC<LinkProps> = ({
	className,
	children,
	size,
	asChild,
	showExternalIcon = true,
	...props
}) => {
	const Comp = asChild ? Slot.Root : "a";
	return (
		<Comp className={cn(linkVariants({ size }), className)} {...props}>
			<Slot.Slottable>{children}</Slot.Slottable>
			{showExternalIcon && <SquareArrowOutUpRightIcon />}
		</Comp>
	);
};
