// Story-level overrides for pixel-storybook's snapshot matrix. These merge onto
// the global matrix from the pixel config, so specifying `viewports` here keeps
// the config's `themes`.

export const pixelWithTablet = {
	viewports: ["tablet", "desktop"],
};

// Desktop only. Use when a story's play function or layout only works above a
// given breakpoint (for example Tailwind `md`), so tablet/phone shots would
// fail or hide the controls under test.
export const pixelWithDesktop = {
	viewports: ["desktop"],
};

// Phone only. Use when a story depends on mobile CSS (for example `sm:hidden`)
// or a mobile viewport; Pixel's default desktop/laptop width would hide those
// controls even if matchMedia is mocked.
export const pixelWithPhone = {
	viewports: ["phone"],
};
