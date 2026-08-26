/**
 * Shared popper animations that preserve Tailwind v3's directional entry offset.
 * The open-state variant keeps Tailwind v4's animate-in reset from overriding it.
 */
export const popperContentAnimationClass = [
	"data-[state=open]:animate-in data-[state=closed]:animate-out",
	"data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0",
	"data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95",
	"data-[state=open]:data-[side=bottom]:slide-in-from-top-2 data-[state=open]:data-[side=left]:slide-in-from-right-2",
	"data-[state=open]:data-[side=right]:slide-in-from-left-2 data-[state=open]:data-[side=top]:slide-in-from-bottom-2",
].join(" ");
