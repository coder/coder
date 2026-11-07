import { Menu as DropdownMenuPrimitive } from "@base-ui/react/menu";
import { cn } from "cn";
import { CheckIcon, ChevronRightIcon } from "lucide-react";
import { MenuPopup } from "#/components/DropdownMenu/MenuPopup";
import {
	menuContentClass,
	menuItemClass,
	menuSeparatorClass,
} from "./menuClasses";

export const DropdownMenu = DropdownMenuPrimitive.Root;

export const DropdownMenuTrigger = DropdownMenuPrimitive.Trigger;

export const DropdownMenuRadioGroup = DropdownMenuPrimitive.RadioGroup;

type DropdownMenuContentProps = Omit<
	React.ComponentProps<typeof MenuPopup>,
	"className" | "onSelect"
> & { className?: string } & Pick<
		React.ComponentProps<typeof DropdownMenuPrimitive.Positioner>,
		"sideOffset" | "alignOffset" | "collisionPadding" | "side" | "align"
	>;

export const DropdownMenuContent: React.FC<DropdownMenuContentProps> = ({
	className,
	side = "bottom",
	align = "center",
	sideOffset = 4,
	alignOffset,
	collisionPadding,
	onCloseAutoFocus,
	...props
}) => (
	<DropdownMenuPrimitive.Portal>
		<DropdownMenuPrimitive.Positioner
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
		</DropdownMenuPrimitive.Positioner>
	</DropdownMenuPrimitive.Portal>
);

type DropdownMenuItemProps = Omit<
	React.ComponentProps<typeof DropdownMenuPrimitive.Item>,
	"className" | "onSelect"
> & { className?: string } & {
	inset?: boolean;
};

export const DropdownMenuItem: React.FC<DropdownMenuItemProps> = ({
	className,
	inset,
	...props
}) => {
	return (
		<DropdownMenuPrimitive.Item
			className={cn(menuItemClass, inset && "pl-8", className)}
			{...props}
		/>
	);
};

type DropdownMenuCheckboxItemProps = Omit<
	React.ComponentProps<typeof DropdownMenuPrimitive.CheckboxItem>,
	"className" | "onSelect"
> & { className?: string };

export const DropdownMenuCheckboxItem: React.FC<
	DropdownMenuCheckboxItemProps
> = ({ className, children, closeOnClick = true, ...props }) => {
	return (
		<DropdownMenuPrimitive.CheckboxItem
			closeOnClick={closeOnClick}
			className={cn(menuItemClass, "relative pr-8", className)}
			{...props}
		>
			{children}
			<span className="absolute right-2 flex h-3.5 w-3.5 items-center justify-center">
				<DropdownMenuPrimitive.CheckboxItemIndicator>
					<CheckIcon className="size-4" />
				</DropdownMenuPrimitive.CheckboxItemIndicator>
			</span>
		</DropdownMenuPrimitive.CheckboxItem>
	);
};

type DropdownMenuRadioItemProps = Omit<
	React.ComponentProps<typeof DropdownMenuPrimitive.RadioItem>,
	"className" | "onSelect"
> & { className?: string };

export const DropdownMenuRadioItem: React.FC<DropdownMenuRadioItemProps> = ({
	className,
	children,
	closeOnClick = true,
	...props
}) => {
	return (
		<DropdownMenuPrimitive.RadioItem
			closeOnClick={closeOnClick}
			className={cn(
				"relative flex cursor-default select-none items-center rounded-sm py-1.5 pr-8 pl-2 text-sm outline-hidden transition-colors",
				"data-highlighted:bg-surface-secondary data-highlighted:text-content-primary data-disabled:pointer-events-none data-disabled:opacity-50",
				"focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-content-link",
				"data-checked:bg-surface-secondary data-checked:text-content-primary",
				"font-medium",
				className,
			)}
			{...props}
		>
			{children}
			<span className="absolute right-2 flex h-3.5 w-3.5 items-center justify-center">
				<DropdownMenuPrimitive.RadioItemIndicator>
					<CheckIcon className="size-4" />
				</DropdownMenuPrimitive.RadioItemIndicator>
			</span>
		</DropdownMenuPrimitive.RadioItem>
	);
};

export const DropdownMenuSub = DropdownMenuPrimitive.SubmenuRoot;

type DropdownMenuSubTriggerProps = Omit<
	React.ComponentProps<typeof DropdownMenuPrimitive.SubmenuTrigger>,
	"className" | "onSelect"
> & { className?: string } & {
	inset?: boolean;
};

export const DropdownMenuSubTrigger: React.FC<DropdownMenuSubTriggerProps> = ({
	className,
	inset,
	children,
	...props
}) => {
	return (
		<DropdownMenuPrimitive.SubmenuTrigger
			className={cn(menuItemClass, inset && "pl-8", className)}
			{...props}
		>
			{children}
			<ChevronRightIcon className="ml-auto size-3.5" />
		</DropdownMenuPrimitive.SubmenuTrigger>
	);
};

type DropdownMenuSubContentProps = Omit<
	React.ComponentProps<typeof MenuPopup>,
	"className" | "onSelect"
> & { className?: string } & Pick<
		React.ComponentProps<typeof DropdownMenuPrimitive.Positioner>,
		"sideOffset" | "alignOffset" | "collisionPadding" | "side" | "align"
	>;

export const DropdownMenuSubContent: React.FC<DropdownMenuSubContentProps> = ({
	className,
	side = "right",
	align = "start",
	sideOffset = 0,
	alignOffset,
	collisionPadding,
	onCloseAutoFocus,
	...props
}) => (
	<DropdownMenuPrimitive.Portal>
		<DropdownMenuPrimitive.Positioner
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
		</DropdownMenuPrimitive.Positioner>
	</DropdownMenuPrimitive.Portal>
);

type DropdownMenuSeparatorProps = Omit<
	React.ComponentProps<typeof DropdownMenuPrimitive.Separator>,
	"className" | "onSelect"
> & { className?: string };

export const DropdownMenuSeparator: React.FC<DropdownMenuSeparatorProps> = ({
	className,
	...props
}) => {
	return (
		<DropdownMenuPrimitive.Separator
			className={cn([menuSeparatorClass], className)}
			{...props}
		/>
	);
};

type DropdownMenuLabelProps = Omit<
	React.ComponentProps<"div">,
	"className" | "onSelect"
> & {
	className?: string;
};

export const DropdownMenuLabel: React.FC<DropdownMenuLabelProps> = ({
	className,
	...props
}) => {
	return (
		<div
			className={cn(
				"px-2 py-1.5 text-xs font-medium text-content-secondary",
				className,
			)}
			{...props}
		/>
	);
};
