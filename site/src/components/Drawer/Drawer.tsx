/**
 * Adapted from shadcn/ui's Drawer, reimplemented on top of Base UI's Dialog
 * primitive instead of the unmaintained `vaul` package. Base UI has no
 * dedicated drawer, so the slide-in behavior is layered on the Dialog parts
 * with the same public API shadcn exposes.
 * @see {@link https://ui.shadcn.com/docs/components/drawer}
 * @see {@link https://base-ui.com/react/components/dialog}
 */
import { Dialog as DialogPrimitive } from "@base-ui-components/react/dialog";
import { createContext, useContext } from "react";
import { cn } from "#/utils/cn";

type DrawerDirection = "top" | "bottom" | "left" | "right";

const DrawerDirectionContext = createContext<DrawerDirection>("right");

type DrawerProps = React.ComponentPropsWithRef<typeof DialogPrimitive.Root> & {
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
	React.ComponentPropsWithRef<typeof DialogPrimitive.Backdrop>
> = ({ className, ...props }) => {
	return (
		<DialogPrimitive.Backdrop
			className={cn(
				`fixed inset-0 z-50 bg-overlay
				data-[open]:animate-in data-[closed]:animate-out
				data-[closed]:fade-out-0 data-[open]:fade-in-0`,
				className,
			)}
			{...props}
		/>
	);
};

const directionClasses: Record<DrawerDirection, string> = {
	top: `inset-x-0 top-0 mb-24 max-h-[80vh] rounded-b-lg border-b border-border
		data-[open]:slide-in-from-top data-[closed]:slide-out-to-top`,
	bottom: `inset-x-0 bottom-0 mt-24 max-h-[80vh] rounded-t-lg border-t border-border
		data-[open]:slide-in-from-bottom data-[closed]:slide-out-to-bottom`,
	right: `inset-y-0 right-0 h-full w-3/4 border-l border-border sm:max-w-sm
		data-[open]:slide-in-from-right data-[closed]:slide-out-to-right`,
	left: `inset-y-0 left-0 h-full w-3/4 border-r border-border sm:max-w-sm
		data-[open]:slide-in-from-left data-[closed]:slide-out-to-left`,
};

export const DrawerContent: React.FC<
	React.ComponentPropsWithRef<typeof DialogPrimitive.Popup>
> = ({ className, children, ...props }) => {
	const direction = useContext(DrawerDirectionContext);
	return (
		<DrawerPortal>
			<DrawerOverlay />
			<DialogPrimitive.Popup
				data-drawer-direction={direction}
				className={cn(
					`group/drawer-content fixed z-50 flex h-auto flex-col bg-surface-primary outline-none
					transition ease-in-out data-[open]:animate-in data-[closed]:animate-out
					data-[closed]:duration-300 data-[open]:duration-500`,
					directionClasses[direction],
					className,
				)}
				{...props}
			>
				<div
					className={`mx-auto mt-4 hidden h-2 w-[100px] shrink-0 rounded-full bg-surface-tertiary
					group-data-[drawer-direction=bottom]/drawer-content:block`}
				/>
				{children}
			</DialogPrimitive.Popup>
		</DrawerPortal>
	);
};

export const DrawerHeader: React.FC<React.ComponentPropsWithRef<"div">> = ({
	className,
	...props
}) => {
	return (
		<div
			className={cn(
				`flex flex-col gap-0.5 p-4
				group-data-[drawer-direction=bottom]/drawer-content:text-center
				group-data-[drawer-direction=top]/drawer-content:text-center
				md:gap-1.5 md:text-left`,
				className,
			)}
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
