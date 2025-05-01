/**
 * Copied from shadc/ui on 11/06/2024
 * @see {@link https://ui.shadcn.com/docs/components/button}
 */
import { Slot } from "@radix-ui/react-slot";
import { type VariantProps, cva } from "class-variance-authority";
import { forwardRef } from "react";
import { cn } from "utils/cn";

const buttonVariants = cva(
	`inline-flex items-center justify-center gap-1 whitespace-nowrap font-sans
	border-solid rounded-md transition-colors
	text-sm font-semibold font-medium cursor-pointer no-underline
	focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-content-link
	disabled:pointer-events-none disabled:text-content-disabled
	[&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg]:p-0.5`,
	{
		variants: {
			variant: {
				default:
					"bg-surface-invert-primary text-content-invert hover:bg-surface-invert-secondary border-none disabled:bg-surface-secondary font-semibold",
				outline:
					"border border-border-default text-content-primary bg-transparent hover:bg-surface-secondary",
				subtle:
					"border-none bg-transparent text-content-secondary hover:text-content-primary",
				destructive:
					"border border-border-destructive text-content-primary bg-surface-destructive hover:bg-transparent disabled:bg-transparent disabled:text-content-disabled font-semibold",
			},

			size: {
				lg: "min-w-20 h-10 px-3 py-2 [&_svg]:size-icon-lg",
				sm: "min-w-20 h-8 px-2 py-1.5 text-xs [&_svg]:size-icon-sm",
				xs: "min-w-8 py-1 px-2 text-2xs rounded-md",
				icon: "size-8 px-1.5 [&_svg]:size-icon-sm",
				"icon-lg": "size-10 px-2 [&_svg]:size-icon-lg",
			},
		},
		defaultVariants: {
			variant: "default",
			size: "lg",
		},
	},
);

export interface ButtonProps
	extends React.ButtonHTMLAttributes<HTMLButtonElement>,
		VariantProps<typeof buttonVariants> {
	asChild?: boolean;
}

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(
	({ className, variant, size, asChild = false, ...props }, ref) => {
		const Comp = asChild ? Slot : "button";
		return (
			<Comp
				{...props}
				ref={ref}
				className={cn(buttonVariants({ variant, size }), className)}
				// Adding default button type to make sure that buttons don't
				// accidentally trigger form actions when clicked. But because
				// this Button component is so polymorphic (it's also used to
				// make <a> elements look like buttons), we can only safely
				// default to adding the prop when we know that we're rendering
				// a real HTML button instead of an arbitrary Slot. Adding the
				// type attribute to any non-buttons will produce invalid HTML
				type={
					props.type === undefined && Comp === "button" ? "button" : props.type
				}
			/>
		);
	},
);
