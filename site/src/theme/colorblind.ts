/**
 * Colorblind-friendly theme registry and resolver.
 *
 * The six concrete themes (`dark`, `light`, plus the four colorblind
 * variants) are the values the rest of the app actually renders.
 * Higher-level preference shapes (sync mode with two slots, the legacy
 * `auto` string) are translated into one of those six names before
 * they reach the `ThemeProvider`.
 *
 * Keeping the concrete set in one place lets the `ThemeProvider`, the
 * embed page's postMessage validator, and the legacy migration helper
 * stay in sync. Adding a new theme requires updating `CONCRETE_THEMES`
 * plus the corresponding entry in `site/src/theme/index.ts`.
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
 * Resolves a stored theme name to the concrete theme that should be
 * rendered. Unknown, empty, or legacy `auto-*` values fall back to the
 * OS color scheme, which matches the pre-existing contract of tolerating
 * stale database values.
 *
 * Callers that want to preserve the semantics of a legacy `auto-*` value
 * (sync mode with colorblind slots) should call `legacyAutoToSync` first
 * and feed the resulting concrete slot to this resolver.
 */
export const resolveThemeName = (
	preference: string | undefined,
	osScheme: "dark" | "light",
): ConcreteThemeName => {
	if (isConcreteThemeName(preference)) {
		return preference;
	}
	return osScheme;
};

/**
 * Minimal "sync mode" shape. Kept in this module (rather than in
 * `themeMode.ts`) because the mapping from legacy auto-family strings to
 * a sync pair is pure data and naturally lives next to the registry.
 */
type LegacyAutoSync = {
	mode: "sync";
	light: ConcreteThemeName;
	dark: ConcreteThemeName;
};

const LEGACY_AUTO_SYNC: Record<string, LegacyAutoSync> = {
	auto: { mode: "sync", light: "light", dark: "dark" },
	"auto-protan-deuter": {
		mode: "sync",
		light: "light-protan-deuter",
		dark: "dark-protan-deuter",
	},
	"auto-tritan": {
		mode: "sync",
		light: "light-tritan",
		dark: "dark-tritan",
	},
};

/**
 * Translates a legacy `auto`, `auto-protan-deuter`, or `auto-tritan`
 * preference into the equivalent sync-mode state. Returns null for any
 * other value (concrete names, empty, undefined, garbage) so the caller
 * can decide how to treat single-mode preferences separately.
 *
 * `auto` is a real legacy value: the pre-dropdown appearance page
 * exposed it as a first-class choice, and the pre-existing
 * `ThemeProvider` resolved it against the OS color scheme. This helper
 * lets us migrate existing stored `auto` preferences to the new sync
 * mode on read without a database migration. The `auto-protan-deuter`
 * and `auto-tritan` entries are paranoid guards in case a client
 * persisted one of those strings against an API that never officially
 * supported them.
 */
export const legacyAutoToSync = (
	preference: string | undefined,
): LegacyAutoSync | null => {
	if (!preference) {
		return null;
	}
	return LEGACY_AUTO_SYNC[preference] ?? null;
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
