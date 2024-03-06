// eslint-disable-next-line no-restricted-imports -- we still use `Theme` as a basis for our actual theme, for now.
import type { Theme as MuiTheme } from "@mui/material/styles";
import type * as monaco from "monaco-editor";
import dark from "./dark";
import darkBlue from "./darkBlue";
import type { NewTheme } from "./experimental";
import type { ExternalImageModeStyles } from "./externalImages";
import light from "./light";
import type { Roles } from "./roles";

export interface Theme extends MuiTheme {
  experimental: NewTheme;
  roles: Roles;
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
