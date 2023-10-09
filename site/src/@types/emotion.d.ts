import type { DefaultTheme as MuiTheme } from "@mui/system";

declare module "@emotion/react" {
  interface Theme extends MuiTheme {}
}
