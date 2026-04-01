/**
 * Copied from shadc/ui on 12/19/2024
 * @see {@link https://ui.shadcn.com/docs/components/dropdown-menu}
 *
 * This component was updated to match the styles from the Figma design:
 * @see {@link https://www.figma.com/design/WfqIgsTFXN2BscBSSyXWF8/Coder-kit?node-id=656-2354&t=CiGt5le3yJEwMH4M-0}
 */

import * as DropdownMenuPrimitive from "@radix-ui/react-dropdown-menu";
import { Check } from "lucide-react";
import { cn } from "utils/cn";

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
					<Check className="h-4 w-4" />
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
			className={cn(["-mx-1 my-2 h-px bg-border"], className)}
			{...props}
		/>
	);
};
