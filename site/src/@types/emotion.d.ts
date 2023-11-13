import type { DefaultTheme as MuiTheme } from "@mui/system";
import type { NewTheme } from "theme/experimental";

declare module "@emotion/react" {
  interface Theme extends MuiTheme {
    experimental: NewTheme;
  }
}
