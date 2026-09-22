/**
 * Enter and exit motion for Radix popper-positioned overlays (menus,
 * popovers, selects, tooltips).
 *
 * Kept deliberately small since these open dozens of times a session: a
 * single fade + slight zoom with an ease-out curve, no slide, growing from
 * the trigger via `--radix-popper-transform-origin`. Exit is faster than
 * enter since nobody watches a menu leave.
 */
export const popperAnimationClass = [
	"origin-(--radix-popper-transform-origin) ease-out",
	"animate-in fade-in-0 zoom-in-[0.97] duration-150",
	"data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=closed]:zoom-out-[0.97] data-[state=closed]:duration-100",
].join(" ");
