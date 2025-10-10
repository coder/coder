/**
 * Copied from shadc/ui on 11/13/2024
 * @see {@link https://ui.shadcn.com/docs/components/dialog}
 */
import * as DialogPrimitive from "@radix-ui/react-dialog";
import { cva, type VariantProps } from "class-variance-authority";
import {
	type ComponentPropsWithoutRef,
	type ElementRef,
	type FC,
	forwardRef,
	type HTMLAttributes,
} from "react";
import { cn } from "utils/cn";

export const Dialog = DialogPrimitive.Root;

export const DialogTrigger = DialogPrimitive.Trigger;

const DialogPortal = DialogPrimitive.Portal;

export const DialogClose = DialogPrimitive.Close;

const DialogOverlay = forwardRef<
	ElementRef<typeof DialogPrimitive.Overlay>,
	ComponentPropsWithoutRef<typeof DialogPrimitive.Overlay>
>(({ className, ...props }, ref) => (
	<DialogPrimitive.Overlay
		ref={ref}
		className={cn(
			`fixed inset-0 z-50 bg-overlay
			data-[state=open]:animate-in data-[state=closed]:animate-out
			data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0`,
			className,
		)}
		{...props}
	/>
));

const dialogVariants = cva(
	`fixed left-[50%] top-[50%] z-50 grid w-full max-w-lg gap-6
	border border-solid bg-surface-primary p-8 shadow-lg duration-200 sm:rounded-lg
	translate-x-[-50%] translate-y-[-50%]
	data-[state=open]:animate-in data-[state=closed]:animate-out
	data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0
	data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95
	data-[state=closed]:slide-out-to-left-1/2 data-[state=closed]:slide-out-to-top-[48%]
	data-[state=open]:slide-in-from-left-1/2 data-[state=open]:slide-in-from-top-[48%]`,
	{
		variants: {
			variant: {
				default: "border-border-primary",
				destructive: "border-border-destructive",
			},
		},
		defaultVariants: {
			variant: "default",
		},
	},
);

interface DialogContentProps
	extends ComponentPropsWithoutRef<typeof DialogPrimitive.Content>,
		VariantProps<typeof dialogVariants> {}

export const DialogContent = forwardRef<
	ElementRef<typeof DialogPrimitive.Content>,
	DialogContentProps
>(({ className, variant, children, ...props }, ref) => (
	<DialogPortal>
		<DialogOverlay />
		<DialogPrimitive.Content
			ref={ref}
			className={cn(dialogVariants({ variant }), className)}
			{...props}
		>
			{children}
		</DialogPrimitive.Content>
	</DialogPortal>
));

export const DialogHeader: FC<HTMLAttributes<HTMLDivElement>> = ({
	className,
	...props
}) => (
	<div
		className={cn(
			"flex flex-col space-y-5 text-center sm:text-left",
			className,
		)}
		{...props}
	/>
);

export const DialogFooter: FC<HTMLAttributes<HTMLDivElement>> = ({
	className,
	...props
}) => (
	<div
		className={cn(
			"flex flex-col-reverse sm:flex-row sm:justify-end sm:space-x-2",
			className,
		)}
		{...props}
	/>
);

export const DialogTitle = forwardRef<
	ElementRef<typeof DialogPrimitive.Title>,
	ComponentPropsWithoutRef<typeof DialogPrimitive.Title>
>(({ className, ...props }, ref) => (
	<DialogPrimitive.Title
		ref={ref}
		className={cn(
			"text-xl m-0 text-content-primary font-semibold leading-none tracking-tight",
			className,
		)}
		{...props}
	/>
));

export const DialogDescription = forwardRef<
	ElementRef<typeof DialogPrimitive.Description>,
	ComponentPropsWithoutRef<typeof DialogPrimitive.Description>
>(({ className, ...props }, ref) => (
	<DialogPrimitive.Description
		ref={ref}
		className={cn("text-sm text-content-secondary font-medium", className)}
		{...props}
	/>
));
