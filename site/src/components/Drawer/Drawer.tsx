/**
 * Copied from shadcn/ui on 08/04/2026
 * @see {@link https://ui.shadcn.com/docs/components/drawer}
 */
import { Drawer as DrawerPrimitive } from "vaul";
import { cn } from "#/utils/cn";

export const Drawer: React.FC<
	React.ComponentPropsWithRef<typeof DrawerPrimitive.Root>
> = ({ shouldScaleBackground = true, ...props }) => {
	return (
		<DrawerPrimitive.Root
			shouldScaleBackground={shouldScaleBackground}
			{...props}
		/>
	);
};

export const DrawerTrigger = DrawerPrimitive.Trigger;

export const DrawerClose = DrawerPrimitive.Close;

const DrawerPortal = DrawerPrimitive.Portal;

const DrawerOverlay: React.FC<
	React.ComponentPropsWithRef<typeof DrawerPrimitive.Overlay>
> = ({ className, ...props }) => {
	return (
		<DrawerPrimitive.Overlay
			className={cn(
				`fixed inset-0 z-50 bg-overlay
				data-[state=open]:animate-in data-[state=closed]:animate-out
				data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0`,
				className,
			)}
			{...props}
		/>
	);
};

export const DrawerContent: React.FC<
	React.ComponentPropsWithRef<typeof DrawerPrimitive.Content>
> = ({ className, children, ...props }) => {
	return (
		<DrawerPortal>
			<DrawerOverlay />
			<DrawerPrimitive.Content
				className={cn(
					"group/drawer-content fixed z-50 flex h-auto flex-col bg-surface-primary outline-none",
					`data-[vaul-drawer-direction=top]:inset-x-0 data-[vaul-drawer-direction=top]:top-0
					data-[vaul-drawer-direction=top]:mb-24 data-[vaul-drawer-direction=top]:max-h-[80vh]
					data-[vaul-drawer-direction=top]:rounded-b-lg data-[vaul-drawer-direction=top]:border-b data-[vaul-drawer-direction=top]:border-border`,
					`data-[vaul-drawer-direction=bottom]:inset-x-0 data-[vaul-drawer-direction=bottom]:bottom-0
					data-[vaul-drawer-direction=bottom]:mt-24 data-[vaul-drawer-direction=bottom]:max-h-[80vh]
					data-[vaul-drawer-direction=bottom]:rounded-t-lg data-[vaul-drawer-direction=bottom]:border-t data-[vaul-drawer-direction=bottom]:border-border`,
					`data-[vaul-drawer-direction=right]:inset-y-0 data-[vaul-drawer-direction=right]:right-0
					data-[vaul-drawer-direction=right]:h-full data-[vaul-drawer-direction=right]:w-3/4
					data-[vaul-drawer-direction=right]:border-l data-[vaul-drawer-direction=right]:border-border
					data-[vaul-drawer-direction=right]:sm:max-w-sm`,
					`data-[vaul-drawer-direction=left]:inset-y-0 data-[vaul-drawer-direction=left]:left-0
					data-[vaul-drawer-direction=left]:h-full data-[vaul-drawer-direction=left]:w-3/4
					data-[vaul-drawer-direction=left]:border-r data-[vaul-drawer-direction=left]:border-border
					data-[vaul-drawer-direction=left]:sm:max-w-sm`,
					className,
				)}
				{...props}
			>
				<div
					className={`mx-auto mt-4 hidden h-2 w-[100px] shrink-0 rounded-full bg-surface-tertiary
					group-data-[vaul-drawer-direction=bottom]/drawer-content:block`}
				/>
				{children}
			</DrawerPrimitive.Content>
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
				group-data-[vaul-drawer-direction=bottom]/drawer-content:text-center
				group-data-[vaul-drawer-direction=top]/drawer-content:text-center
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
	React.ComponentPropsWithRef<typeof DrawerPrimitive.Title>
> = ({ className, ...props }) => {
	return (
		<DrawerPrimitive.Title
			className={cn(
				"text-lg font-semibold leading-none tracking-tight text-content-primary",
				className,
			)}
			{...props}
		/>
	);
};

export const DrawerDescription: React.FC<
	React.ComponentPropsWithRef<typeof DrawerPrimitive.Description>
> = ({ className, ...props }) => {
	return (
		<DrawerPrimitive.Description
			className={cn("text-sm text-content-secondary", className)}
			{...props}
		/>
	);
};
