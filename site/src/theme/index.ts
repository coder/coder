import type { Theme as MuiTheme } from "@mui/material/styles";
import dark from "./dark";
import darkBlue from "./darkBlue";
import type { NewTheme } from "./experimental";
import type { Colors } from "./colors";

export interface Theme extends MuiTheme {
  colors: Colors;
  experimental: NewTheme;
}

export const DEFAULT_THEME = "auto";

const theme = {
  dark,
  darkBlue,
  light: dark,
} satisfies Record<string, Theme>;

export default theme;
