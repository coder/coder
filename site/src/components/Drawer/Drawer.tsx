import { Dialog as DialogPrimitive } from "@base-ui/react/dialog";
import { cva } from "class-variance-authority";
import { cn } from "cn";
import { createContext, useContext } from "react";
import {
	DialogPopup,
	type DialogPopupProps,
} from "#/components/Dialog/DialogPopup";

type DrawerDirection = "top" | "bottom" | "left" | "right";

const DrawerDirectionContext = createContext<DrawerDirection>("right");

type DrawerProps = React.ComponentProps<typeof DialogPrimitive.Root> & {
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

type DrawerOverlayProps = Omit<
	React.ComponentProps<typeof DialogPrimitive.Backdrop>,
	"className"
> & {
	className?: string;
};

const DrawerOverlay: React.FC<DrawerOverlayProps> = ({
	className,
	...props
}) => {
	return (
		<DialogPrimitive.Backdrop
			className={cn(
				"fixed inset-0 z-50 bg-overlay ease-out-strong",
				"data-open:animate-in data-closed:animate-out",
				"data-closed:fade-out-0 data-open:fade-in-0",
				"data-open:duration-300 data-closed:duration-100",
				className,
			)}
			{...props}
		/>
	);
};

const drawerContentVariants = cva(
	cn(
		"fixed z-50 flex h-auto flex-col bg-surface-tertiary outline-hidden will-change-transform ease-out-strong",
		"data-open:animate-in data-closed:animate-out",
		"data-open:duration-500 data-closed:duration-300",
	),
	{
		variants: {
			direction: {
				top: cn(
					"inset-x-0 top-0 max-h-[80vh] w-full border-b border-border",
					"data-open:slide-in-from-top data-closed:slide-out-to-top",
				),
				bottom: cn(
					"inset-x-0 bottom-0 max-h-[80vh] w-full border-t border-border",
					"data-open:slide-in-from-bottom data-closed:slide-out-to-bottom",
				),
				left: cn(
					"inset-y-0 left-0 h-full w-3/4 border-r border-border sm:max-w-sm",
					"data-open:slide-in-from-left data-closed:slide-out-to-left",
				),
				right: cn(
					"inset-y-0 right-0 h-full w-3/4 border-l border-border sm:max-w-sm",
					"data-open:slide-in-from-right data-closed:slide-out-to-right",
				),
			},
		},
		defaultVariants: {
			direction: "right",
		},
	},
);

export const DrawerContent: React.FC<DialogPopupProps> = ({
	className,
	children,
	...props
}) => {
	const direction = useContext(DrawerDirectionContext);

	return (
		<DrawerPortal>
			<DrawerOverlay />
			<DialogPopup
				className={cn(drawerContentVariants({ direction }), className)}
				{...props}
			>
				{children}
			</DialogPopup>
		</DrawerPortal>
	);
};

export const DrawerHeader: React.FC<React.ComponentProps<"div">> = ({
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

export const DrawerFooter: React.FC<React.ComponentProps<"div">> = ({
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

type DrawerTitleProps = Omit<
	React.ComponentProps<typeof DialogPrimitive.Title>,
	"className"
> & {
	className?: string;
};

export const DrawerTitle: React.FC<DrawerTitleProps> = ({
	className,
	...props
}) => {
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

type DrawerDescriptionProps = Omit<
	React.ComponentProps<typeof DialogPrimitive.Description>,
	"className"
> & { className?: string };

export const DrawerDescription: React.FC<DrawerDescriptionProps> = ({
	className,
	...props
}) => {
	return (
		<DialogPrimitive.Description
			className={cn("text-sm text-content-secondary", className)}
			{...props}
		/>
	);
};
