/**
 * Copied from shadc/ui on 11/06/2024
 * @see {@link https://ui.shadcn.com/docs/components/button}
 */
import { Slot } from "@radix-ui/react-slot";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "utils/cn";

// Be careful when changing the child styles from the button such as images
// because they can override the styles from other components like Avatar.
export const buttonVariants = cva(
	`
	group inline-flex items-center justify-center gap-1 whitespace-nowrap font-sans
	border-solid rounded-md transition-colors shrink-0
	text-sm font-medium cursor-pointer no-underline
	focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-content-link
	disabled:pointer-events-none disabled:text-content-disabled
	[&:is(a):not([href])]:pointer-events-none [&:is(a):not([href])]:text-content-disabled
	[&>svg]:pointer-events-none [&>svg]:shrink-0 [&>svg]:p-0.5
	[&>img]:pointer-events-none [&>img]:shrink-0 [&>img]:p-0.5
	`,
	{
		variants: {
			variant: {
				default: `
					border-none bg-surface-invert-primary font-semibold text-content-invert
					hover:bg-surface-invert-secondary
					disabled:bg-surface-secondary
					`,
				outline: `
					border border-border-default bg-transparent text-content-primary
					hover:bg-surface-secondary
					`,
				subtle: `
					border-none bg-transparent text-content-secondary
					hover:text-content-primary
					`,
				destructive: `
					border border-border-destructive font-semibold text-content-primary bg-surface-destructive
					hover:bg-transparent
					disabled:bg-transparent disabled:text-content-disabled
					`,
			},

			size: {
				lg: "min-w-20 h-10 px-3 py-2 [&>svg]:size-icon-lg [&>img]:size-icon-lg",
				sm: "min-w-20 h-8 px-2 py-1.5 text-xs [&>svg]:size-icon-sm [&>img]:size-icon-sm",
				xs: "min-w-8 py-1 px-2 text-2xs rounded-md",
				icon: "size-8 px-1.5 [&>svg]:size-icon-sm [&>img]:size-icon-sm",
				"icon-lg": "size-10 px-2 [&>svg]:size-icon-lg [&>img]:size-icon-lg",
			},
		},
		defaultVariants: {
			variant: "default",
			size: "lg",
		},
	},
);

export type ButtonProps = React.ComponentPropsWithRef<"button"> &
	VariantProps<typeof buttonVariants> & {
		asChild?: boolean;
	};

export const Button: React.FC<ButtonProps> = ({
	className,
	variant,
	size,
	asChild = false,
	...props
}) => {
	const Comp = asChild ? Slot : "button";

	// We want `type` to default to `"button"` when the component is not being
	// used as a `Slot`. The default behavior of any given `<button>` element is
	// to submit the closest parent `<form>` because Web Platform reasons. This
	// prevents that. However, we don't want to set it on non-`<button>`s when
	// `asChild` is set.
	// https://developer.mozilla.org/en-US/docs/Web/HTML/Reference/Elements/button#type
	if (!asChild && !props.type) {
		props.type = "button";
	}

	return (
		<Comp
			{...props}
			className={cn(buttonVariants({ variant, size }), className)}
		/>
	);
};
