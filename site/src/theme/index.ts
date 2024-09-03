// biome-ignore lint/nursery/noRestrictedImports: We still use `Theme` as a basis for our actual theme, for now.
import type { Theme as MuiTheme } from "@mui/material/styles";
import type * as monaco from "monaco-editor";
import type { Roles } from "./roles";
import dark from "./dark";
import darkBlue from "./darkBlue";
import type { NewTheme } from "./experimental";
import type { ExternalImageModeStyles } from "./externalImages";
import light from "./light";

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
	monaco: monaco.editor.IStandaloneThemeData;
	externalImages: ExternalImageModeStyles;
}

export const DEFAULT_THEME = "dark";

const theme = {
	dark,
	darkBlue,
	light,
} satisfies Record<string, Theme>;

export default theme;
