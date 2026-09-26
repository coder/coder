/**
 * Adapted from `DropdownMenu.tsx` to wrap Radix's ContextMenu primitive.
 * Shares menu styling with DropdownMenu via `menuClasses.ts` so the
 * click-triggered and right-click-triggered menus stay in visual sync
 * by construction.
 * @see {@link https://www.radix-ui.com/primitives/docs/components/context-menu}
 */

import { cn } from "cn";
import { ChevronRightIcon } from "lucide-react";
import { ContextMenu as ContextMenuPrimitive } from "radix-ui";
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
	React.ComponentProps<typeof ContextMenuPrimitive.Content>
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

type ContextMenuItemProps = React.ComponentProps<
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

export const ContextMenuSub = ContextMenuPrimitive.Sub;

export const ContextMenuSubTrigger: React.FC<
	React.ComponentProps<typeof ContextMenuPrimitive.SubTrigger>
> = ({ className, children, ...props }) => {
	return (
		<ContextMenuPrimitive.SubTrigger
			className={cn(menuItemClass, className)}
			{...props}
		>
			{children}
			<ChevronRightIcon className="ml-auto size-3.5" />
		</ContextMenuPrimitive.SubTrigger>
	);
};

export const ContextMenuSubContent: React.FC<
	React.ComponentProps<typeof ContextMenuPrimitive.SubContent>
> = ({ className, ...props }) => {
	return (
		<ContextMenuPrimitive.Portal>
			<ContextMenuPrimitive.SubContent
				className={cn(menuContentClass, className)}
				{...props}
			/>
		</ContextMenuPrimitive.Portal>
	);
};

export const ContextMenuSeparator: React.FC<
	React.ComponentProps<typeof ContextMenuPrimitive.Separator>
> = ({ className, ...props }) => {
	return (
		<ContextMenuPrimitive.Separator
			className={cn([menuSeparatorClass], className)}
			{...props}
		/>
	);
};
