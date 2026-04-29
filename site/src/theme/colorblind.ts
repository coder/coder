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

export const isConcreteThemeName = (
	value: unknown,
): value is ConcreteThemeName => {
	return typeof value === "string" && concreteThemeSet.has(value);
};

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

export const baseModeFor = (
	concreteName: ConcreteThemeName,
): "dark" | "light" => {
	return concreteName.startsWith("dark") ? "dark" : "light";
};
