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
import {
	menuContentClass,
	menuItemClass,
	menuSeparatorClass,
} from "./menuClasses";

/**
 * Non-modal by default: a modal menu sets `aria-hidden` on the rest of the
 * page while its focusable controls stay in the tab order, which axe reports
 * as aria-hidden-focus.
 */
export const DropdownMenu: React.FC<
	React.ComponentProps<typeof DropdownMenuPrimitive.Root>
> = ({ modal = false, ...props }) => {
	return <DropdownMenuPrimitive.Root modal={modal} {...props} />;
};

export const DropdownMenuTrigger = DropdownMenuPrimitive.Trigger;

export const DropdownMenuRadioGroup = DropdownMenuPrimitive.RadioGroup;

const DIALOG_SELECTOR = '[role="dialog"], [role="alertdialog"]';

/**
 * Stops the click that ends a press, so dismissing a non-modal menu does not
 * also activate what is under the pointer, such as a clickable table row.
 */
const swallowNextClick = (doc: Document) => {
	// A press that never becomes a click, such as a right click or a touch
	// scroll, must not swallow a later click.
	const endEvents = [
		"pointerdown",
		"pointercancel",
		"contextmenu",
		"keydown",
	] as const;
	const release = () => {
		doc.removeEventListener("click", swallow, true);
		for (const type of endEvents) {
			doc.removeEventListener(type, release, true);
		}
	};
	const swallow = (event: MouseEvent) => {
		event.preventDefault();
		event.stopPropagation();
		release();
	};
	doc.addEventListener("click", swallow, true);
	for (const type of endEvents) {
		doc.addEventListener(type, release, true);
	}
};

export const DropdownMenuContent: React.FC<
	React.ComponentProps<typeof DropdownMenuPrimitive.Content>
> = ({
	className,
	sideOffset = 4,
	onFocusOutside,
	onPointerDownOutside,
	...props
}) => {
	return (
		<DropdownMenuPrimitive.Portal>
			<DropdownMenuPrimitive.Content
				sideOffset={sideOffset}
				className={cn(menuContentClass, className)}
				onPointerDownOutside={(event) => {
					onPointerDownOutside?.(event);
					const { target } = event.detail.originalEvent;
					const doc = target instanceof Node ? target.ownerDocument : null;
					if (event.defaultPrevented || !(target instanceof Node) || !doc) {
						return;
					}
					// A dialog or popover the press lands outside dismisses on the same
					// click, and Radix keeps it open when that click is stopped.
					if (
						[...doc.querySelectorAll(DIALOG_SELECTOR)].every((dialog) =>
							dialog.contains(target),
						)
					) {
						swallowNextClick(doc);
					}
				}}
				// An item that opens a dialog moves focus into it. Keep that from
				// counting as an outside interaction, so focus returns to the
				// trigger when the dialog closes.
				onFocusOutside={(event) => {
					onFocusOutside?.(event);
					if (
						event.target instanceof Element &&
						event.target.closest(DIALOG_SELECTOR)
					) {
						event.preventDefault();
					}
				}}
				{...props}
			/>
		</DropdownMenuPrimitive.Portal>
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
	return (
		<DropdownMenuPrimitive.Item
			className={cn(menuItemClass, inset && "pl-8", className)}
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
			<span className="absolute right-2 flex size-3.5 items-center justify-center">
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
				"focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-content-link",
				"data-[state=checked]:bg-surface-secondary data-[state=checked]:text-content-primary",
				"font-medium",
				className,
			)}
			{...props}
		>
			{children}
			<span className="absolute right-2 flex size-3.5 items-center justify-center">
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

export const DropdownMenuLabel: React.FC<
	React.ComponentProps<typeof DropdownMenuPrimitive.Label>
> = ({ className, ...props }) => {
	return (
		<DropdownMenuPrimitive.Label
			className={cn(
				"px-2 py-1.5 text-xs font-medium text-content-secondary",
				className,
			)}
			{...props}
		/>
	);
};
