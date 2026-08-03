// Story-level overrides for pixel-storybook's snapshot matrix. These merge onto
// the global matrix from the pixel config, so specifying `viewports` here keeps
// the config's `themes`.

export const pixelWithTablet = {
	viewports: ["tablet", "desktop"],
};
