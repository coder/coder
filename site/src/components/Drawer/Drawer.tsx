import { cva } from "class-variance-authority";
import { Dialog as DialogPrimitive } from "radix-ui";
import { createContext, useContext } from "react";
import { cn } from "#/utils/cn";

type DrawerDirection = "top" | "bottom" | "left" | "right";

const DrawerDirectionContext = createContext<DrawerDirection>("right");

type DrawerProps = React.ComponentPropsWithRef<typeof DialogPrimitive.Root> & {
	/** The edge of the screen the drawer slides in from. Defaults to "right". */
	direction?: DrawerDirection;
};

export const Drawer: React.FC<DrawerProps> = ({
	direction = "right",
	...props
}) => {
	return (
		<DrawerDirectionContext.Provider value={direction}>
			<DialogPrimitive.Root {...props} />
		</DrawerDirectionContext.Provider>
	);
};

export const DrawerTrigger = DialogPrimitive.Trigger;

export const DrawerClose = DialogPrimitive.Close;

const DrawerPortal = DialogPrimitive.Portal;

const DrawerOverlay: React.FC<
	React.ComponentPropsWithRef<typeof DialogPrimitive.Overlay>
> = ({ className, ...props }) => {
	return (
		<DialogPrimitive.Overlay
			className={cn(
				"fixed inset-0 z-50 bg-overlay",
				"data-[state=open]:animate-in data-[state=closed]:animate-out",
				"data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0",
				"data-[state=open]:duration-300 data-[state=closed]:duration-100",
				className,
			)}
			{...props}
		/>
	);
};

const drawerContentVariants = cva(
	`fixed z-50 flex h-auto flex-col bg-surface-primary outline-none will-change-transform
	data-[state=open]:animate-in data-[state=closed]:animate-out
	data-[state=open]:duration-500 data-[state=closed]:duration-300`,
	{
		variants: {
			direction: {
				top: `inset-x-0 top-0 max-h-[80vh] w-full border-b border-border
					data-[state=open]:slide-in-from-top data-[state=closed]:slide-out-to-top`,
				bottom: `inset-x-0 bottom-0 max-h-[80vh] w-full border-t border-border
					data-[state=open]:slide-in-from-bottom data-[state=closed]:slide-out-to-bottom`,
				left: `inset-y-0 left-0 h-full w-3/4 border-r border-border sm:max-w-sm
					data-[state=open]:slide-in-from-left data-[state=closed]:slide-out-to-left`,
				right: `inset-y-0 right-0 h-full w-3/4 border-l border-border sm:max-w-sm
					data-[state=open]:slide-in-from-right data-[state=closed]:slide-out-to-right`,
			},
		},
		defaultVariants: {
			direction: "right",
		},
	},
);

export const DrawerContent: React.FC<
	React.ComponentPropsWithRef<typeof DialogPrimitive.Content>
> = ({ className, children, ...props }) => {
	const direction = useContext(DrawerDirectionContext);

	return (
		<DrawerPortal>
			<DrawerOverlay />
			<DialogPrimitive.Content
				className={cn(drawerContentVariants({ direction }), className)}
				{...props}
			>
				{children}
			</DialogPrimitive.Content>
		</DrawerPortal>
	);
};

export const DrawerHeader: React.FC<React.ComponentPropsWithRef<"div">> = ({
	className,
	...props
}) => {
	return (
		<div
			className={cn("flex flex-col gap-0.5 p-4 md:gap-1.5", className)}
			{...props}
		/>
	);
};

export const DrawerFooter: React.FC<React.ComponentPropsWithRef<"div">> = ({
	className,
	...props
}) => {
	return (
		<div
			className={cn("mt-auto flex flex-col gap-2 p-4", className)}
			{...props}
		/>
	);
};

export const DrawerTitle: React.FC<
	React.ComponentPropsWithRef<typeof DialogPrimitive.Title>
> = ({ className, ...props }) => {
	return (
		<DialogPrimitive.Title
			className={cn(
				"text-lg font-semibold leading-none tracking-tight text-content-primary",
				className,
			)}
			{...props}
		/>
	);
};

export const DrawerDescription: React.FC<
	React.ComponentPropsWithRef<typeof DialogPrimitive.Description>
> = ({ className, ...props }) => {
	return (
		<DialogPrimitive.Description
			className={cn("text-sm text-content-secondary", className)}
			{...props}
		/>
	);
};
