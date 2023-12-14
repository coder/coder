import { type Theme as MuiTheme } from "@mui/material/styles";
import dark from "./dark";
import darkBlue from "./darkBlue";
import { type NewTheme } from "./experimental";
import { Colors } from "./colors";

export interface Theme extends MuiTheme {
  colors: Colors;
  experimental: NewTheme;
}

export const DEFAULT_THEME = "auto";

const theme = {
  dark,
  darkBlue,
  light: darkBlue,
} satisfies Record<string, Theme>;

export default theme;
