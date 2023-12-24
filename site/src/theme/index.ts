import type { Theme as MuiTheme } from "@mui/material/styles";
import type * as monaco from "monaco-editor";
import dark from "./dark";
import darkBlue from "./darkBlue";
import light from "./light";
import type { NewTheme } from "./experimental";

export interface Theme extends MuiTheme {
  experimental: NewTheme;
  monaco: monaco.editor.IStandaloneThemeData;
}

export const DEFAULT_THEME = "dark";

const theme = {
  dark,
  darkBlue,
  light,
} satisfies Record<string, Theme>;

export default theme;
