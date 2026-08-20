/**
 * Returns `true` when the viewport width is at or below the `sm`
 * Tailwind breakpoint (< 640 px), which is a reasonable proxy for a
 * mobile / touch device where auto-focusing an input would cause the
 * virtual keyboard to pop up unexpectedly.
 */
export const isMobileViewport = (): boolean => {
	return window.matchMedia("(max-width: 639px)").matches;
};

/**
 * Builds a `useSyncExternalStore` subscribe function that notifies on
 * changes to the given media query, so every viewport hook shares one
 * listener lifecycle implementation.
 */
export const createMediaQuerySubscribe =
	(query: string) =>
	(onStoreChange: () => void): (() => void) => {
		const mediaQuery = window.matchMedia(query);
		mediaQuery.addEventListener("change", onStoreChange);
		return () => mediaQuery.removeEventListener("change", onStoreChange);
	};

export const belowMdViewportMediaQuery = "(max-width: 767px)";

/**
 * Returns `true` when the viewport width is below the `md` Tailwind
 * breakpoint (< 768 px). Use this for layout branching that needs to
 * align with `md:` Tailwind utilities (e.g. the mobile full-width
 * dropdown / inline menu layout), so that viewports between 640 and
 * 767 px (common on landscape phones and small tablets) pick the
 * mobile branch instead of the desktop flyout branch.
 */
export const isBelowMdViewport = (): boolean => {
	return window.matchMedia(belowMdViewportMediaQuery).matches;
};

export const belowLgViewportMediaQuery = "(max-width: 1023px)";

/**
 * Returns `true` when the viewport width is below the `lg` Tailwind
 * breakpoint (< 1024 px). Use this to align with `lg:` Tailwind
 * utilities that switch between a side-by-side layout and a
 * single-panel-at-a-time layout (e.g. the Agents chat page's chat vs.
 * right panel split).
 */
export const isBelowLgViewport = (): boolean => {
	return window.matchMedia(belowLgViewportMediaQuery).matches;
};
