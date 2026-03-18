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
