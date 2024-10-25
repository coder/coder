// biome-ignore lint/nursery/noRestrictedImports: We still use `Theme` as a basis for our actual theme, for now.
import type { Theme as MuiTheme } from "@mui/material/styles";
import type * as monaco from "monaco-editor";
import type { Branding } from "./branding";
import dark from "./dark";
import type { NewTheme } from "./experimental";
import type { ExternalImageModeStyles } from "./externalImages";
import light from "./light";
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

const theme = {
	dark,
	light,
} satisfies Record<string, Theme>;

export default theme;
