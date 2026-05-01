import { CONCRETE_THEMES, type ConcreteThemeName } from "#/theme";

/**
 * Display copy for each concrete theme. The descriptions are surfaced
 * under each tile in single-theme mode. Text is adapted from the
 * GitHub appearance preferences, with "Coder" substituted for "GitHub"
 * to match the product.
 */
type ThemeCopy = {
	title: string;
	description: string;
	beta?: boolean;
};

export const THEME_COPY: Record<ConcreteThemeName, ThemeCopy> = {
	light: {
		title: "Light default",
		description:
			"Coder's standard light theme with full color contrast and brightness.",
	},
	"light-protan-deuter": {
		title: "Light protanopia and deuteranopia",
		description:
			"For people who may find it difficult to distinguish between reds and greens.",
		beta: true,
	},
	"light-tritan": {
		title: "Light tritanopia",
		description:
			"For people who find it difficult to distinguish between blues and greens, as well as yellows and purples.",
		beta: true,
	},
	dark: {
		title: "Dark default",
		description:
			"Coder's standard dark theme with full color contrast and brightness on a dark background.",
	},
	"dark-protan-deuter": {
		title: "Dark protanopia and deuteranopia",
		description:
			"For people who may find it difficult to distinguish between reds and greens, with a dark background.",
		beta: true,
	},
	"dark-tritan": {
		title: "Dark tritanopia",
		description:
			"For people who find it difficult to distinguish between blues and greens, as well as yellows and purples, with a dark background.",
		beta: true,
	},
};

/**
 * Concrete theme names grouped by OS color scheme. The order matches
 * the in-product taxonomy: default first, then colorblind variants.
 */
export const LIGHT_THEMES: ConcreteThemeName[] = [
	"light",
	"light-protan-deuter",
	"light-tritan",
];

export const DARK_THEMES: ConcreteThemeName[] = [
	"dark",
	"dark-protan-deuter",
	"dark-tritan",
];

export const SYNC_MODE_THEMES: ConcreteThemeName[] = [
	...LIGHT_THEMES,
	...DARK_THEMES,
];

const syncModeThemes = SYNC_MODE_THEMES;
const themeCopyKeys = Object.keys(THEME_COPY);
if (
	syncModeThemes.length !== CONCRETE_THEMES.length ||
	themeCopyKeys.length !== CONCRETE_THEMES.length ||
	!CONCRETE_THEMES.every((theme) => syncModeThemes.includes(theme))
) {
	throw new Error(
		"Theme copy registries are out of sync with CONCRETE_THEMES.",
	);
}
