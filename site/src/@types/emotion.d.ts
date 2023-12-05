import type { Theme as MuiTheme } from "@mui/material/styles";

declare module "@emotion/react" {
  interface Theme extends MuiTheme {}
}
