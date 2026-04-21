/**
 * Copied from shadc/ui on 12/19/2024
 * @see {@link https://ui.shadcn.com/docs/components/dropdown-menu}
 *
 * This component was updated to match the styles from the Figma design:
 * @see {@link https://www.figma.com/design/WfqIgsTFXN2BscBSSyXWF8/Coder-kit?node-id=656-2354&t=CiGt5le3yJEwMH4M-0}
 */
import { CheckIcon } from "lucide-react";
import { DropdownMenu as DropdownMenuPrimitive } from "radix-ui";
import { cn } from "#/utils/cn";
import {
	menuContentClass,
	menuItemClass,
	menuSeparatorClass,
} from "./menuClasses";

export const DropdownMenu = DropdownMenuPrimitive.Root;

export const DropdownMenuTrigger = DropdownMenuPrimitive.Trigger;

export const DropdownMenuGroup = DropdownMenuPrimitive.Group;

export const DropdownMenuRadioGroup = DropdownMenuPrimitive.RadioGroup;

export const DropdownMenuContent: React.FC<
	React.ComponentPropsWithRef<typeof DropdownMenuPrimitive.Content>
> = ({ className, sideOffset = 4, ...props }) => {
	return (
		<DropdownMenuPrimitive.Portal>
			<DropdownMenuPrimitive.Content
				sideOffset={sideOffset}
				className={cn(menuContentClass, className)}
				{...props}
			/>
		</DropdownMenuPrimitive.Portal>
	);
};

type DropdownMenuItemProps = React.ComponentPropsWithRef<
	typeof DropdownMenuPrimitive.Item
> & {
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

export const DropdownMenuRadioItem: React.FC<
	React.ComponentPropsWithRef<typeof DropdownMenuPrimitive.RadioItem>
> = ({ className, children, ...props }) => {
	return (
		<DropdownMenuPrimitive.RadioItem
			className={cn(
				"relative flex cursor-default select-none items-center rounded-sm py-1.5 pr-8 pl-2 text-sm outline-none transition-colors",
				"focus:bg-surface-secondary focus:text-content-primary data-[disabled]:pointer-events-none data-[disabled]:opacity-50",
				"data-[state=checked]:bg-surface-secondary data-[state=checked]:text-content-primary",
				"font-medium",
				className,
			)}
			{...props}
		>
			{children}
			<span className="absolute top-3.5 right-2 flex h-3.5 w-3.5 items-center justify-center">
				<DropdownMenuPrimitive.ItemIndicator>
					<CheckIcon className="h-4 w-4" />
				</DropdownMenuPrimitive.ItemIndicator>
			</span>
		</DropdownMenuPrimitive.RadioItem>
	);
};

export const DropdownMenuSeparator: React.FC<
	React.ComponentPropsWithRef<typeof DropdownMenuPrimitive.Separator>
> = ({ className, ...props }) => {
	return (
		<DropdownMenuPrimitive.Separator
			className={cn([menuSeparatorClass], className)}
			{...props}
		/>
	);
};
