import {
	type ExternalImageModeStyles,
	forDarkThemes,
	forLightThemes,
} from "./externalImages";

export type Theme = {
	mode: "dark" | "light";
	externalImages: ExternalImageModeStyles;
};

export const DEFAULT_THEME = "dark";

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

export const legacyAutoToSync = (
	preference: string | undefined,
): LegacyAutoSync | null => {
	if (!preference) {
		return null;
	}
	return LEGACY_AUTO_SYNC[preference] ?? null;
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

const dark: Theme = { mode: "dark", externalImages: forDarkThemes };
const light: Theme = { mode: "light", externalImages: forLightThemes };

const theme = {
	dark,
	light,
	"dark-protan-deuter": dark,
	"light-protan-deuter": light,
	"dark-tritan": dark,
	"light-tritan": light,
} satisfies Record<ConcreteThemeName, Theme>;

export default theme;
