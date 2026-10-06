/** Anchors a popover to the composer's bottom edge without publishing geometry. */
export const composerMenuAnchor = (composer: HTMLElement) => ({
	contextElement: composer,
	getBoundingClientRect: () => {
		const { left, bottom, width } = composer.getBoundingClientRect();
		return new DOMRect(left, bottom, width, 0);
	},
});
