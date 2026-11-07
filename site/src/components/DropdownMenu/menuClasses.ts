export const menuContentClass = [
	"z-50 outline-hidden min-w-48 overflow-hidden rounded-md border border-solid bg-surface-primary p-2 text-content-secondary shadow-md",
	"origin-(--transform-origin) ease-out-strong",
	"data-open:animate-in data-closed:animate-out data-closed:fade-out-0 data-open:fade-in-0",
	"data-closed:zoom-out-95 data-open:zoom-in-95",
	"data-open:data-[side=bottom]:slide-in-from-top-2 data-open:data-[side=left]:slide-in-from-right-2",
	"data-open:data-[side=right]:slide-in-from-left-2 data-open:data-[side=top]:slide-in-from-bottom-2",
].join(" ");

export const menuItemClass = `
	relative flex cursor-default select-none items-center gap-2 rounded-sm
	px-2 py-1.5 text-sm text-content-secondary font-medium outline-hidden
	no-underline
	data-highlighted:bg-surface-secondary data-highlighted:text-content-primary
	focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-content-link
	data-disabled:pointer-events-none data-disabled:opacity-50
	[&>svg]:size-icon-sm [&>svg]:shrink-0
	[&>img]:size-icon-sm [&>img]:shrink-0
	`;

export const menuSeparatorClass = "-mx-1 my-2 h-px bg-border";
