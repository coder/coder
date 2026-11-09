import { ContextMenu as ContextMenuPrimitive } from "@base-ui/react/context-menu";
import { Menu } from "@base-ui/react/menu";
import { cn } from "cn";
import { ChevronRightIcon } from "lucide-react";
import { MenuPopup } from "#/components/DropdownMenu/MenuPopup";
import {
	menuContentClass,
	menuItemClass,
	menuSeparatorClass,
} from "../DropdownMenu/menuClasses";

export const ContextMenu = ContextMenuPrimitive.Root;

export const ContextMenuTrigger = ContextMenuPrimitive.Trigger;

const stopRightClick = (event: React.MouseEvent) => {
	if (event.button === 2) {
		event.preventDefault();
		event.stopPropagation();
	}
};

/**
 * Spread on a menu button inside a row that has its own context menu, so a
 * right-click on the button does not also open the row's context menu.
 */
export const preventRowContextMenuProps = {
	onContextMenuCapture: (event: React.MouseEvent) => {
		event.preventDefault();
		event.stopPropagation();
	},
	onMouseDownCapture: stopRightClick,
	onPointerDownCapture: stopRightClick,
};

/** @public */
export const ContextMenuGroup = ContextMenuPrimitive.Group;

/** @public */
export const ContextMenuRadioGroup = ContextMenuPrimitive.RadioGroup;

type ContextMenuContentProps = Omit<
	React.ComponentProps<typeof MenuPopup>,
	"className" | "onSelect"
> & { className?: string } & Pick<
		React.ComponentProps<typeof ContextMenuPrimitive.Positioner>,
		"sideOffset" | "alignOffset" | "collisionPadding"
	>;

export const ContextMenuContent: React.FC<ContextMenuContentProps> = ({
	className,
	sideOffset = 4,
	alignOffset,
	collisionPadding,
	onCloseAutoFocus,
	...props
}) => (
	<ContextMenuPrimitive.Portal>
		<ContextMenuPrimitive.Positioner
			sideOffset={sideOffset}
			alignOffset={alignOffset}
			collisionPadding={collisionPadding}
			data-menu-positioner
			className="z-50 outline-hidden"
		>
			<MenuPopup
				className={cn(menuContentClass, className)}
				onCloseAutoFocus={onCloseAutoFocus}
				{...props}
			/>
		</ContextMenuPrimitive.Positioner>
	</ContextMenuPrimitive.Portal>
);

type ContextMenuItemProps = Omit<
	React.ComponentProps<typeof ContextMenuPrimitive.Item>,
	"className" | "onSelect"
> & { className?: string } & {
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

export const ContextMenuSub = ContextMenuPrimitive.SubmenuRoot;

type ContextMenuSubTriggerProps = Omit<
	React.ComponentProps<typeof ContextMenuPrimitive.SubmenuTrigger>,
	"className" | "onSelect"
> & { className?: string } & {
	inset?: boolean;
};

export const ContextMenuSubTrigger: React.FC<ContextMenuSubTriggerProps> = ({
	className,
	inset,
	children,
	...props
}) => {
	return (
		<ContextMenuPrimitive.SubmenuTrigger
			className={cn(menuItemClass, inset && "pl-8", className)}
			{...props}
		>
			{children}
			<ChevronRightIcon className="ml-auto size-3.5" />
		</ContextMenuPrimitive.SubmenuTrigger>
	);
};

type ContextMenuSubContentProps = Omit<
	React.ComponentProps<typeof MenuPopup>,
	"className" | "onSelect"
> & { className?: string } & Pick<
		React.ComponentProps<typeof Menu.Positioner>,
		"sideOffset" | "alignOffset" | "collisionPadding" | "side" | "align"
	>;

export const ContextMenuSubContent: React.FC<ContextMenuSubContentProps> = ({
	className,
	side = "right",
	align = "start",
	sideOffset = 0,
	alignOffset,
	collisionPadding,
	onCloseAutoFocus,
	...props
}) => (
	<ContextMenuPrimitive.Portal>
		<Menu.Positioner
			side={side}
			align={align}
			sideOffset={sideOffset}
			alignOffset={alignOffset}
			collisionPadding={collisionPadding}
			data-menu-positioner
			className="z-50 outline-hidden"
		>
			<MenuPopup
				className={cn(menuContentClass, className)}
				onCloseAutoFocus={onCloseAutoFocus}
				{...props}
			/>
		</Menu.Positioner>
	</ContextMenuPrimitive.Portal>
);

type ContextMenuSeparatorProps = Omit<
	React.ComponentProps<typeof ContextMenuPrimitive.Separator>,
	"className" | "onSelect"
> & { className?: string };

export const ContextMenuSeparator: React.FC<ContextMenuSeparatorProps> = ({
	className,
	...props
}) => {
	return (
		<ContextMenuPrimitive.Separator
			className={cn([menuSeparatorClass], className)}
			{...props}
		/>
	);
};
