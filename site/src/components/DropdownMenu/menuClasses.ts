import { popperContentAnimationClass } from "#/components/Popper/popperClasses";

export const menuContentClass = [
	"z-50 min-w-48 overflow-hidden rounded-md border border-solid bg-surface-primary p-2 text-content-secondary shadow-md",
	popperContentAnimationClass,
].join(" ");

export const menuItemClass = `
	relative flex cursor-default select-none items-center gap-2 rounded-sm
	px-2 py-1.5 text-sm text-content-secondary font-medium outline-hidden
	no-underline
	focus:bg-surface-secondary focus:text-content-primary
	data-disabled:pointer-events-none data-disabled:opacity-50
	[&>svg]:size-icon-sm [&>svg]:shrink-0
	[&>img]:size-icon-sm [&>img]:shrink-0
	`;

export const menuSeparatorClass = "-mx-1 my-2 h-px bg-border";
