/**
 * Colorblind-friendly theme registry and resolver.
 *
 * The four colorblind variants (`dark-protan-deuter`, `light-protan-deuter`,
 * `dark-tritan`, `light-tritan`) sit alongside the standard `dark` and
 * `light` themes. The legacy `auto` preference is virtual: it resolves to
 * `dark` or `light` at runtime based on the user's OS color scheme.
 *
 * Keeping the list of concrete themes in one place lets the `ThemeProvider`
 * and the embed page's postMessage validator stay in sync. Adding a new
 * theme requires updating `CONCRETE_THEMES` plus the corresponding entry
 * in `site/src/theme/index.ts`.
 */

export const CONCRETE_THEMES = [
	"dark",
	"light",
	"dark-protan-deuter",
	"light-protan-deuter",
	"dark-tritan",
	"light-tritan",
] as const;

export type ConcreteThemeName = (typeof CONCRETE_THEMES)[number];

const concreteThemeSet = new Set<string>(CONCRETE_THEMES);

/**
 * Returns true when the given value is one of the concrete theme names.
 * Used by `AgentEmbedPage` to validate incoming `postMessage` payloads,
 * which must supply a concrete theme because embeds do not observe OS
 * color-scheme changes on their own.
 */
export const isConcreteThemeName = (
	value: unknown,
): value is ConcreteThemeName => {
	return typeof value === "string" && concreteThemeSet.has(value);
};

/**
 * Resolves a stored `theme_preference` value to the concrete theme that
 * should be rendered. The legacy `auto` preference maps to the OS color
 * scheme. Unknown or empty values fall back to the OS scheme, which
 * matches the behavior of the previous `ThemeProvider` implementation
 * and tolerates stale database values from older clients.
 */
export const resolveThemeName = (
	preference: string | undefined,
	osScheme: "dark" | "light",
): ConcreteThemeName => {
	if (preference === "auto") {
		return osScheme;
	}
	if (isConcreteThemeName(preference)) {
		return preference;
	}
	return osScheme;
};

/**
 * Returns the base mode class (`dark` or `light`) that should be applied
 * alongside a concrete theme class. ThemeProvider and AgentEmbedPage set
 * both classes on `<html>` so Tailwind's `dark:` variant (configured as
 * `darkMode: ["selector"]`) and any selector-based theming keyed on
 * `.dark` or `.light` keep matching when a colorblind variant is the
 * concrete theme.
 */
export const baseModeFor = (
	concreteName: ConcreteThemeName,
): "dark" | "light" => {
	return concreteName.startsWith("dark") ? "dark" : "light";
};
