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
import { createContext, useContext, useEffect, useRef, useState } from "react";
import {
	menuContentClass,
	menuItemClass,
	menuSeparatorClass,
} from "./menuClasses";

/**
 * Blocks the page below popovers and dialogs while a non-modal menu is open,
 * so the press that dismisses the menu does not reach the element under it.
 * Not a React portal, so ancestor popovers see presses on it as outside.
 */
const useOutsideClickCatcher = (active: boolean) => {
	useEffect(() => {
		if (!active) {
			return;
		}
		const catcher = document.createElement("div");
		catcher.className = "fixed inset-0 z-49";
		catcher.dataset.testid = "dropdown-menu-click-catcher";

		let pressed = false;
		let closed = false;

		const onPointerDown = (event: PointerEvent) => {
			if (event.button !== 0) {
				return;
			}
			pressed = true;
			// Keeps the click on the catcher if the pointer is released elsewhere.
			catcher.setPointerCapture(event.pointerId);
		};
		// Touch hit-tests a tap on release, so outlive the menu until the
		// press that closed it ends.
		const onPressEnd = () => {
			pressed = false;
			if (closed) {
				catcher.remove();
			}
		};

		catcher.addEventListener("pointerdown", onPointerDown);
		catcher.addEventListener("click", onPressEnd);
		catcher.addEventListener("pointercancel", onPressEnd);
		document.body.append(catcher);

		return () => {
			closed = true;
			if (!pressed) {
				catcher.remove();
			}
		};
	}, [active]);
};

const TriggerRefContext =
	createContext<React.RefObject<HTMLButtonElement | null> | null>(null);

/**
 * Non-modal by default: a modal menu sets `aria-hidden` on the rest of the
 * page while its focusable controls stay in the tab order, which axe reports
 * as aria-hidden-focus.
 */
export const DropdownMenu: React.FC<
	React.ComponentProps<typeof DropdownMenuPrimitive.Root>
> = ({
	modal = false,
	open: openProp,
	defaultOpen = false,
	onOpenChange,
	...props
}) => {
	const [uncontrolledOpen, setUncontrolledOpen] = useState(defaultOpen);
	const open = openProp ?? uncontrolledOpen;
	const triggerRef = useRef<HTMLButtonElement>(null);
	useOutsideClickCatcher(open && !modal);
	return (
		<TriggerRefContext value={triggerRef}>
			<DropdownMenuPrimitive.Root
				modal={modal}
				open={open}
				onOpenChange={(next) => {
					setUncontrolledOpen(next);
					onOpenChange?.(next);
				}}
				{...props}
			/>
		</TriggerRefContext>
	);
};

export const DropdownMenuTrigger: React.FC<
	React.ComponentProps<typeof DropdownMenuPrimitive.Trigger>
> = ({ onPointerDown, onKeyDown, ...props }) => {
	const triggerRef = useContext(TriggerRefContext);
	// Records the trigger on the interactions that open the menu.
	const record = (event: React.SyntheticEvent<HTMLButtonElement>) => {
		if (triggerRef) {
			triggerRef.current = event.currentTarget;
		}
	};
	return (
		<DropdownMenuPrimitive.Trigger
			onPointerDown={(event) => {
				record(event);
				onPointerDown?.(event);
			}}
			onKeyDown={(event) => {
				record(event);
				onKeyDown?.(event);
			}}
			{...props}
		/>
	);
};

export const DropdownMenuRadioGroup = DropdownMenuPrimitive.RadioGroup;

const DIALOG_SELECTOR = '[role="dialog"], [role="alertdialog"]';

export const DropdownMenuContent: React.FC<
	React.ComponentProps<typeof DropdownMenuPrimitive.Content>
> = ({
	className,
	sideOffset = 4,
	onFocusOutside,
	onCloseAutoFocus,
	...props
}) => {
	const triggerRef = useContext(TriggerRefContext);
	return (
		<DropdownMenuPrimitive.Portal>
			<DropdownMenuPrimitive.Content
				sideOffset={sideOffset}
				className={cn(menuContentClass, className)}
				// A press on the click catcher leaves focus on the body. Return it to
				// the trigger, as a modal menu does.
				onCloseAutoFocus={(event) => {
					onCloseAutoFocus?.(event);
					if (
						!event.defaultPrevented &&
						document.activeElement === document.body
					) {
						triggerRef?.current?.focus();
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
