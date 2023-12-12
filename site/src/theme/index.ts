import { type Theme as MuiTheme } from "@mui/material/styles";
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
} satisfies Record<string, Theme>;

export default theme;
