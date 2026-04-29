/**
 * Adapted from `DropdownMenu.tsx` to wrap Radix's ContextMenu primitive.
 * Shares menu styling with DropdownMenu via `menuClasses.ts` so the
 * click-triggered and right-click-triggered menus stay in visual sync
 * by construction.
 * @see {@link https://www.radix-ui.com/primitives/docs/components/context-menu}
 */
import { ContextMenu as ContextMenuPrimitive } from "radix-ui";
import { cn } from "#/utils/cn";
import {
	menuContentClass,
	menuItemClass,
	menuSeparatorClass,
} from "../DropdownMenu/menuClasses";

export const ContextMenu = ContextMenuPrimitive.Root;

export const ContextMenuTrigger = ContextMenuPrimitive.Trigger;

/** @public */
export const ContextMenuGroup = ContextMenuPrimitive.Group;

/** @public */
export const ContextMenuRadioGroup = ContextMenuPrimitive.RadioGroup;

export const ContextMenuContent: React.FC<
	React.ComponentPropsWithRef<typeof ContextMenuPrimitive.Content>
> = ({ className, ...props }) => {
	return (
		<ContextMenuPrimitive.Portal>
			<ContextMenuPrimitive.Content
				className={cn(menuContentClass, className)}
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
			className={cn(menuItemClass, inset && "pl-8", className)}
			{...props}
		/>
	);
};

export const ContextMenuSeparator: React.FC<
	React.ComponentPropsWithRef<typeof ContextMenuPrimitive.Separator>
> = ({ className, ...props }) => {
	return (
		<ContextMenuPrimitive.Separator
			className={cn([menuSeparatorClass], className)}
			{...props}
		/>
	);
};
