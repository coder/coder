/**
 * Copied from shadc/ui on 11/13/2024
 * @see {@link https://ui.shadcn.com/docs/components/dialog}
 */
import * as DialogPrimitive from "@radix-ui/react-dialog";
import { X } from "lucide-react";
import {
	type ComponentPropsWithoutRef,
	type ElementRef,
	type FC,
	type HTMLAttributes,
	forwardRef,
} from "react";
import { cn } from "utils/cn";

export const Dialog = DialogPrimitive.Root;

export const DialogTrigger = DialogPrimitive.Trigger;

export const DialogPortal = DialogPrimitive.Portal;

export const DialogClose = DialogPrimitive.Close;

export const DialogOverlay = forwardRef<
	ElementRef<typeof DialogPrimitive.Overlay>,
	ComponentPropsWithoutRef<typeof DialogPrimitive.Overlay>
>(({ className, ...props }, ref) => (
	<DialogPrimitive.Overlay
		ref={ref}
		className={cn(
			`fixed inset-0 z-50 bg-black/80
			data-[state=open]:animate-in data-[state=closed]:animate-out
			data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0`,
			className,
		)}
		{...props}
	/>
));

export const DialogContent = forwardRef<
	ElementRef<typeof DialogPrimitive.Content>,
	ComponentPropsWithoutRef<typeof DialogPrimitive.Content>
>(({ className, children, ...props }, ref) => (
	<DialogPortal>
		<DialogOverlay />
		<DialogPrimitive.Content
			ref={ref}
			className={cn(
				`fixed left-[50%] top-[50%] z-50 grid w-full max-w-lg
				gap-4 border bg-background p-6 shadow-lg duration-200 sm:rounded-lg
				translate-x-[-50%] translate-y-[-50%]
				data-[state=open]:animate-in data-[state=closed]:animate-out
				data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0
				data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95
				data-[state=closed]:slide-out-to-left-1/2 data-[state=closed]:slide-out-to-top-[48%]
				data-[state=open]:slide-in-from-left-1/2 data-[state=open]:slide-in-from-top-[48%]`,
				className,
			)}
			{...props}
		>
			{children}
			<DialogPrimitive.Close
				className={`absolute right-4 top-4 rounded-sm opacity-70 ring-offset-background transition-opacity
			hover:opacity-100
			focus:outline-none focus:ring-2 focus:ring-ring focus:ring-offset-2
			disabled:pointer-events-none
			data-[state=open]:bg-surface-secondary data-[state=open]:text-content-disabled`}
			>
				<X className="h-4 w-4" />
				<span className="sr-only">Close</span>
			</DialogPrimitive.Close>
		</DialogPrimitive.Content>
	</DialogPortal>
));

export const DialogHeader: FC<HTMLAttributes<HTMLDivElement>> = ({
	className,
	...props
}) => (
	<div
		className={cn(
			"flex flex-col space-y-1.5 text-center sm:text-left",
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
			"text-lg font-semibold leading-none tracking-tight",
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
		className={cn("text-sm text-content-disabled", className)}
		{...props}
	/>
));
