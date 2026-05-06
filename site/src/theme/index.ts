/** @deprecated MUI Theme type is deprecated. Migrate to Tailwind CSS theme system. */
import type { Theme as MuiTheme } from "@mui/material/styles";
import type * as monaco from "monaco-editor";
import type { Branding } from "./branding";
import dark from "./dark";
import darkProtanDeuter from "./darkProtanDeuter";
import darkTritan from "./darkTritan";
import type { NewTheme } from "./experimental";
import type { ExternalImageModeStyles } from "./externalImages";
import light from "./light";
import lightProtanDeuter from "./lightProtanDeuter";
import lightTritan from "./lightTritan";
import type { Roles } from "./roles";

export interface Theme extends Omit<MuiTheme, "palette"> {
	/** @deprecated prefer `theme.roles` when possible */
	palette: MuiTheme["palette"];

	/** Sets of colors that can be used based on the role that a UI element serves
	 * for the user.
	 * Does it signify an error? a warning? that something is currently running? etc.
	 */
	roles: Roles;

	/** Theme properties that we're testing out but haven't committed to. */
	experimental: NewTheme;

	/** Theme colors related to marketing */
	branding: Branding;

	monaco: monaco.editor.IStandaloneThemeData;
	externalImages: ExternalImageModeStyles;
}

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

const theme = {
	dark,
	light,
	"dark-protan-deuter": darkProtanDeuter,
	"light-protan-deuter": lightProtanDeuter,
	"dark-tritan": darkTritan,
	"light-tritan": lightTritan,
} satisfies Record<ConcreteThemeName, Theme>;

export default theme;
