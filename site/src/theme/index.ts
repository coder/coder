import { type Theme as MuiTheme } from "@mui/material";
import dark from "./dark";
import darkBlue from "./darkBlue";
import { type NewTheme } from "./experimental";
import { Colors } from "./colors";

export interface Theme extends MuiTheme {
  colors: Colors;
  experimental: NewTheme;
}

const theme = {
  dark,
  darkBlue,
  light: dark,
} satisfies Record<string, Theme>;

export default theme;
