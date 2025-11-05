/**
 * Sheet component for slide-out panels
 * Based on shadcn/ui Sheet component
 * @see {@link https://ui.shadcn.com/docs/components/sheet}
 */
import * as DialogPrimitive from "@radix-ui/react-dialog";
import { cva, type VariantProps } from "class-variance-authority";
import { XIcon } from "lucide-react";
import {
	type ComponentPropsWithoutRef,
	type ElementRef,
	forwardRef,
	type HTMLAttributes,
} from "react";
import { cn } from "utils/cn";

export const Sheet = DialogPrimitive.Root;

const SheetPortal = DialogPrimitive.Portal;

const SheetOverlay = forwardRef<
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
SheetOverlay.displayName = DialogPrimitive.Overlay.displayName;

const sheetVariants = cva(
	`fixed z-50 gap-6 bg-surface-primary p-6 shadow-lg transition ease-in-out
	data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:duration-300
	data-[state=open]:duration-500`,
	{
		variants: {
			side: {
				top: `inset-x-0 top-0 border-b border-border-primary
					data-[state=closed]:slide-out-to-top data-[state=open]:slide-in-from-top`,
				bottom: `inset-x-0 bottom-0 border-t border-border-primary
					data-[state=closed]:slide-out-to-bottom data-[state=open]:slide-in-from-bottom`,
				left: `inset-y-0 left-0 h-full w-3/4 border-r border-border-primary sm:max-w-xl
					data-[state=closed]:slide-out-to-left data-[state=open]:slide-in-from-left`,
				right: `inset-y-0 right-0 h-full w-3/4 border-l border-border-primary sm:max-w-xl
					data-[state=closed]:slide-out-to-right data-[state=open]:slide-in-from-right`,
			},
		},
		defaultVariants: {
			side: "right",
		},
	},
);

interface SheetContentProps
	extends ComponentPropsWithoutRef<typeof DialogPrimitive.Content>,
		VariantProps<typeof sheetVariants> {}

export const SheetContent = forwardRef<
	ElementRef<typeof DialogPrimitive.Content>,
	SheetContentProps
>(({ side = "right", className, children, ...props }, ref) => (
	<SheetPortal>
		<SheetOverlay />
		<DialogPrimitive.Content
			ref={ref}
			className={cn(sheetVariants({ side }), className)}
			{...props}
		>
			{children}
			<DialogPrimitive.Close className="absolute right-4 top-4 rounded-sm opacity-70 ring-offset-surface-primary transition-opacity hover:opacity-100 focus:outline-none focus:ring-2 focus:ring-border-focus focus:ring-offset-2 disabled:pointer-events-none data-[state=open]:bg-surface-secondary">
				<XIcon className="size-icon-md" />
				<span className="sr-only">Close</span>
			</DialogPrimitive.Close>
		</DialogPrimitive.Content>
	</SheetPortal>
));
SheetContent.displayName = DialogPrimitive.Content.displayName;

export const SheetHeader = ({
	className,
	...props
}: HTMLAttributes<HTMLDivElement>) => (
	<div
		className={cn(
			"flex flex-col space-y-2 text-center sm:text-left",
			className,
		)}
		{...props}
	/>
);
SheetHeader.displayName = "SheetHeader";

export const SheetTitle = forwardRef<
	ElementRef<typeof DialogPrimitive.Title>,
	ComponentPropsWithoutRef<typeof DialogPrimitive.Title>
>(({ className, ...props }, ref) => (
	<DialogPrimitive.Title
		ref={ref}
		className={cn("text-lg font-semibold text-content-primary", className)}
		{...props}
	/>
));
SheetTitle.displayName = DialogPrimitive.Title.displayName;

export const SheetDescription = forwardRef<
	ElementRef<typeof DialogPrimitive.Description>,
	ComponentPropsWithoutRef<typeof DialogPrimitive.Description>
>(({ className, ...props }, ref) => (
	<DialogPrimitive.Description
		ref={ref}
		className={cn("text-sm text-content-secondary", className)}
		{...props}
	/>
));
SheetDescription.displayName = DialogPrimitive.Description.displayName;
