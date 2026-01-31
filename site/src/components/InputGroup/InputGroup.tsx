import { cva, type VariantProps } from "class-variance-authority";
import { Button, type ButtonProps } from "components/Button/Button";
import { Input } from "components/Input/Input";
import { type FC, forwardRef } from "react";
import { cn } from "utils/cn";

const InputGroup: FC<React.ComponentProps<"div">> = ({
	className,
	...props
}) => {
	return (
		<div
			role="group"
			className={cn(
				// Base styles
				"group/input-group relative flex h-10 w-full min-w-0 items-center rounded-md border border-solid border-border bg-transparent transition-colors outline-none",
				// Focus-visible ring when input inside is focused
				"has-[input:focus-visible]:ring-2 has-[input:focus-visible]:ring-content-link",
				// Invalid state
				"has-[input[aria-invalid=true]]:border-border-destructive",
				// Disabled state
				"has-[input:disabled]:opacity-50 has-[input:disabled]:cursor-not-allowed",
				className,
			)}
			{...props}
		/>
	);
};

const inputGroupAddonVariants = cva(
	"text-content-secondary h-auto gap-2 text-sm font-medium flex cursor-text items-center justify-center select-none group-has-[input:disabled]/input-group:opacity-50 [&>svg:not([class*='size-'])]:size-4",
	{
		variants: {
			align: {
				"inline-start": "pl-3 pr-2 order-first",
				"inline-end": "pl-1 pr-1.5 order-last",
			},
		},
		defaultVariants: {
			align: "inline-start",
		},
	},
);

const InputGroupAddon: FC<
	React.ComponentProps<"div"> & VariantProps<typeof inputGroupAddonVariants>
> = ({ className, align = "inline-start", ...props }) => {
	return (
		// biome-ignore lint/a11y/useKeyWithClickEvents: Click focuses the input, keyboard users can tab directly to input.
		<div
			data-align={align}
			className={cn(inputGroupAddonVariants({ align }), className)}
			onClick={(e) => {
				if ((e.target as HTMLElement).closest("button")) {
					return;
				}
				e.currentTarget.parentElement
					?.querySelector<HTMLInputElement>("input")
					?.focus();
			}}
			{...props}
		/>
	);
};

const InputGroupInput = forwardRef<
	HTMLInputElement,
	React.ComponentProps<typeof Input>
>(({ className, ...props }, ref) => {
	return (
		<Input
			ref={ref}
			className={cn(
				// Reset Input's default styles that conflict with group
				"flex-1 rounded-none border-0 bg-transparent shadow-none ring-0 focus-visible:ring-0 disabled:bg-transparent aria-invalid:ring-0",
				// Adjust padding based on addon position
				"group-has-[[data-align=inline-start]]/input-group:pl-0",
				"group-has-[[data-align=inline-end]]/input-group:pr-0",
				className,
			)}
			{...props}
		/>
	);
});

const InputGroupButton: FC<ButtonProps> = ({
	className,
	size = "sm",
	variant = "subtle",
	...props
}) => {
	return (
		<Button
			size={size}
			variant={variant}
			className={cn(
				// Override styles for fitting within the group
				"min-w-0 rounded-[calc(var(--radius)-3px)]",
				className,
			)}
			{...props}
		/>
	);
};

export { InputGroup, InputGroupAddon, InputGroupInput, InputGroupButton };
