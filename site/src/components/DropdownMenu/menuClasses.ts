export const menuContentClass = [
	"z-50 min-w-48 overflow-hidden rounded-md border border-solid bg-surface-primary p-2 text-content-secondary shadow-md",
	"origin-(--radix-popper-transform-origin) ease-out-strong",
	"data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0",
	"data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95",
	"data-[state=open]:data-[side=bottom]:slide-in-from-top-2 data-[state=open]:data-[side=left]:slide-in-from-right-2",
	"data-[state=open]:data-[side=right]:slide-in-from-left-2 data-[state=open]:data-[side=top]:slide-in-from-bottom-2",
].join(" ");

/**
 * Keyboard focus indicator shared by every menu item. A background change
 * alone is not enough because checked radio items already use the focus
 * background. The ring is inset so scrollable menu containers cannot clip it.
 */
export const menuItemFocusClass =
	"focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-content-link";

export const menuItemClass = `
	relative flex cursor-default select-none items-center gap-2 rounded-sm
	px-2 py-1.5 text-sm text-content-secondary font-medium outline-hidden
	no-underline
	focus:bg-surface-secondary focus:text-content-primary
	${menuItemFocusClass}
	data-disabled:pointer-events-none data-disabled:opacity-50
	[&>svg]:size-icon-sm [&>svg]:shrink-0
	[&>img]:size-icon-sm [&>img]:shrink-0
	`;

export const menuSeparatorClass = "-mx-1 my-2 h-px bg-border";
