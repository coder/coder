/**
 * Styled after the DropdownMenu component, wrapping Radix's ContextMenu
 * primitive so the visual language matches across click-triggered and
 * right-click-triggered menus.
 * @see {@link https://www.radix-ui.com/primitives/docs/components/context-menu}
 */
import { ContextMenu as ContextMenuPrimitive } from "radix-ui";
import { cn } from "#/utils/cn";

export const ContextMenu = ContextMenuPrimitive.Root;

export const ContextMenuTrigger = ContextMenuPrimitive.Trigger;

// eslint-disable-next-line @typescript-eslint/no-unused-vars
export const ContextMenuGroup = ContextMenuPrimitive.Group;

// eslint-disable-next-line @typescript-eslint/no-unused-vars
export const ContextMenuRadioGroup = ContextMenuPrimitive.RadioGroup;

export const ContextMenuContent: React.FC<
	React.ComponentPropsWithRef<typeof ContextMenuPrimitive.Content>
> = ({ className, ...props }) => {
	return (
		<ContextMenuPrimitive.Portal>
			<ContextMenuPrimitive.Content
				className={cn(
					"z-50 min-w-48 overflow-hidden rounded-md border border-solid bg-surface-primary p-2 text-content-secondary shadow-md",
					"data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0",
					"data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95",
					"data-[side=bottom]:slide-in-from-top-2 data-[side=left]:slide-in-from-right-2",
					"data-[side=right]:slide-in-from-left-2 data-[side=top]:slide-in-from-bottom-2",
					className,
				)}
				{...props}
			/>
		</ContextMenuPrimitive.Portal>
	);
};

type ContextMenuItemProps = React.ComponentPropsWithRef<
	typeof ContextMenuPrimitive.Item
> & {
	inset?: boolean;
};

export const ContextMenuItem: React.FC<ContextMenuItemProps> = ({
	className,
	inset,
	...props
}) => {
	return (
		<ContextMenuPrimitive.Item
			className={cn(
				`
				relative flex cursor-default select-none items-center gap-2 rounded-sm
				px-2 py-1.5 text-sm text-content-secondary font-medium outline-none
				no-underline
				focus:bg-surface-secondary focus:text-content-primary
				data-[disabled]:pointer-events-none data-[disabled]:opacity-50
				[&_svg]:size-icon-sm [&>svg]:shrink-0
				[&_img]:size-icon-sm [&>img]:shrink-0
				`,
				inset && "pl-8",
				className,
			)}
			{...props}
		/>
	);
};

export const ContextMenuSeparator: React.FC<
	React.ComponentPropsWithRef<typeof ContextMenuPrimitive.Separator>
> = ({ className, ...props }) => {
	return (
		<ContextMenuPrimitive.Separator
			className={cn(["-mx-1 my-2 h-px bg-border"], className)}
			{...props}
		/>
	);
};
