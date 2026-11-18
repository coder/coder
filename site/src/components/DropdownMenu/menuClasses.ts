import { cn } from "cn";

/**
 * Height cap bounded by the space Radix reports between the trigger and the
 * viewport edge. Menus that use it can scroll, so every item stays reachable
 * on short viewports and at 200% browser zoom. `FilterCombobox` also applies
 * it to each panel inside its `overflow-visible` popup, so flyouts can extend
 * past the popup.
 */
export const menuMaxHeightClassName = "max-h-(--radix-popper-available-height)";

export const menuContentClass = cn(
	"z-50 min-w-[min(12rem,var(--radix-popper-available-width))] max-w-(--radix-popper-available-width) rounded-md border bg-surface-primary p-2 text-content-secondary shadow-md",
	menuMaxHeightClassName,
	"overflow-y-auto overflow-x-hidden",
	"origin-(--radix-popper-transform-origin) ease-out-strong",
	"data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0",
	"data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95",
	"data-[state=open]:data-[side=bottom]:slide-in-from-top-2 data-[state=open]:data-[side=left]:slide-in-from-right-2",
	"data-[state=open]:data-[side=right]:slide-in-from-left-2 data-[state=open]:data-[side=top]:slide-in-from-bottom-2",
);

/**
 * Account for p-2 padding and the 1px border so submenu rows align with
 * their trigger and adjacent menus are separated by 4px.
 */
export const menuSubContentOffsets = {
	sideOffset: 13,
	alignOffset: -9,
};

export const menuItemClass = cn(
	"relative flex cursor-default select-none items-center gap-2 rounded-sm",
	"px-2 py-1.5 text-sm text-content-secondary font-medium outline-hidden",
	"no-underline",
	"focus:bg-surface-secondary focus:text-content-primary",
	"focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-content-link",
	"data-disabled:pointer-events-none data-disabled:opacity-50",
	"aria-disabled:opacity-50",
	"data-[state=open]:bg-surface-secondary",
	"[&>svg]:size-icon-sm [&>svg]:shrink-0",
	"[&>img]:size-icon-sm [&>img]:shrink-0",
);

export const menuSeparatorClass = "-mx-1 my-2 h-px bg-border";
