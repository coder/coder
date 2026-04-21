/**
 * Returns `true` when the viewport width is at or below the `sm`
 * Tailwind breakpoint (< 640 px), which is a reasonable proxy for a
 * mobile / touch device where auto-focusing an input would cause the
 * virtual keyboard to pop up unexpectedly.
 */
export const isMobileViewport = (): boolean => {
	if (typeof window === "undefined" || !window.matchMedia) {
		return false;
	}
	return window.matchMedia("(max-width: 639px)").matches;
};

/**
 * Returns `true` when the viewport width is below the `md` Tailwind
 * breakpoint (< 768 px). Use this for layout branching that needs to
 * align with `md:` Tailwind utilities (e.g. the mobile full-width
 * dropdown / inline menu layout), so that viewports between 640 and
 * 767 px (common on landscape phones and small tablets) pick the
 * mobile branch instead of the desktop flyout branch.
 */
export const isBelowMdViewport = (): boolean => {
	if (typeof window === "undefined" || !window.matchMedia) {
		return false;
	}
	return window.matchMedia("(max-width: 767px)").matches;
};
