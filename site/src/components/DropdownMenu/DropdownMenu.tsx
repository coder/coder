/**
 * Copied from shadc/ui on 12/19/2024
 * @see {@link https://ui.shadcn.com/docs/components/dropdown-menu}
 *
 * This component was updated to match the styles from the Figma design:
 * @see {@link https://www.figma.com/design/WfqIgsTFXN2BscBSSyXWF8/Coder-kit?node-id=656-2354&t=CiGt5le3yJEwMH4M-0}
 */

import { cn } from "cn";
import { CheckIcon, ChevronRightIcon } from "lucide-react";
import { DropdownMenu as DropdownMenuPrimitive } from "radix-ui";
import { createContext, useContext } from "react";
import {
	menuContentClass,
	menuContentSmallClass,
	menuItemClass,
	menuItemSmallClass,
	menuLabelClass,
	menuLabelSmallClass,
	menuSeparatorClass,
} from "./menuClasses";

export const DropdownMenu = DropdownMenuPrimitive.Root;

export const DropdownMenuTrigger = DropdownMenuPrimitive.Trigger;

export const DropdownMenuRadioGroup = DropdownMenuPrimitive.RadioGroup;

type MenuSize = "default" | "sm";

const MenuSizeContext = createContext<MenuSize>("default");

export const DropdownMenuContent: React.FC<
	React.ComponentProps<typeof DropdownMenuPrimitive.Content> & {
		/** Compact variant with extra-small item text. */
		size?: MenuSize;
	}
> = ({ className, sideOffset = 4, size = "default", ...props }) => {
	return (
		<MenuSizeContext.Provider value={size}>
			<DropdownMenuPrimitive.Portal>
				<DropdownMenuPrimitive.Content
					sideOffset={sideOffset}
					className={cn(
						menuContentClass,
						size === "sm" && menuContentSmallClass,
						className,
					)}
					{...props}
				/>
			</DropdownMenuPrimitive.Portal>
		</MenuSizeContext.Provider>
	);
};

export const DropdownMenuLabel: React.FC<
	React.ComponentProps<typeof DropdownMenuPrimitive.Label>
> = ({ className, ...props }) => {
	const size = useContext(MenuSizeContext);
	return (
		<DropdownMenuPrimitive.Label
			className={cn(
				menuLabelClass,
				size === "sm" && menuLabelSmallClass,
				className,
			)}
			{...props}
		/>
	);
};

type DropdownMenuItemProps = React.ComponentProps<
	typeof DropdownMenuPrimitive.Item
> & {
	inset?: boolean;
};

export const DropdownMenuItem: React.FC<DropdownMenuItemProps> = ({
	className,
	inset,
	...props
}) => {
	const size = useContext(MenuSizeContext);
	return (
		<DropdownMenuPrimitive.Item
			className={cn(
				menuItemClass,
				size === "sm" && menuItemSmallClass,
				inset && "pl-8",
				className,
			)}
			{...props}
		/>
	);
};

export const DropdownMenuCheckboxItem: React.FC<
	React.ComponentProps<typeof DropdownMenuPrimitive.CheckboxItem>
> = ({ className, children, ...props }) => {
	return (
		<DropdownMenuPrimitive.CheckboxItem
			className={cn(menuItemClass, "relative pr-8", className)}
			{...props}
		>
			{children}
			<span className="absolute right-2 flex h-3.5 w-3.5 items-center justify-center">
				<DropdownMenuPrimitive.ItemIndicator>
					<CheckIcon className="size-4" />
				</DropdownMenuPrimitive.ItemIndicator>
			</span>
		</DropdownMenuPrimitive.CheckboxItem>
	);
};

export const DropdownMenuRadioItem: React.FC<
	React.ComponentProps<typeof DropdownMenuPrimitive.RadioItem>
> = ({ className, children, ...props }) => {
	return (
		<DropdownMenuPrimitive.RadioItem
			className={cn(
				"relative flex cursor-default select-none items-center rounded-sm py-1.5 pr-8 pl-2 text-sm outline-hidden transition-colors",
				"focus:bg-surface-secondary focus:text-content-primary data-disabled:pointer-events-none data-disabled:opacity-50",
				"data-[state=checked]:bg-surface-secondary data-[state=checked]:text-content-primary",
				"font-medium",
				className,
			)}
			{...props}
		>
			{children}
			<span className="absolute right-2 flex h-3.5 w-3.5 items-center justify-center">
				<DropdownMenuPrimitive.ItemIndicator>
					<CheckIcon className="size-4" />
				</DropdownMenuPrimitive.ItemIndicator>
			</span>
		</DropdownMenuPrimitive.RadioItem>
	);
};

export const DropdownMenuSub = DropdownMenuPrimitive.Sub;

export const DropdownMenuSubTrigger: React.FC<
	React.ComponentProps<typeof DropdownMenuPrimitive.SubTrigger> & {
		inset?: boolean;
	}
> = ({ className, inset, children, ...props }) => {
	return (
		<DropdownMenuPrimitive.SubTrigger
			className={cn(menuItemClass, inset && "pl-8", className)}
			{...props}
		>
			{children}
			<ChevronRightIcon className="ml-auto size-3.5" />
		</DropdownMenuPrimitive.SubTrigger>
	);
};

export const DropdownMenuSubContent: React.FC<
	React.ComponentProps<typeof DropdownMenuPrimitive.SubContent>
> = ({ className, ...props }) => {
	return (
		<DropdownMenuPrimitive.Portal>
			<DropdownMenuPrimitive.SubContent
				className={cn(menuContentClass, className)}
				{...props}
			/>
		</DropdownMenuPrimitive.Portal>
	);
};

export const DropdownMenuSeparator: React.FC<
	React.ComponentProps<typeof DropdownMenuPrimitive.Separator>
> = ({ className, ...props }) => {
	return (
		<DropdownMenuPrimitive.Separator
			className={cn([menuSeparatorClass], className)}
			{...props}
		/>
	);
};
