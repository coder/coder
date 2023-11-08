import type { DefaultTheme as MuiTheme } from "@mui/system";
import type { CoderTheme } from "theme/theme";

declare module "@emotion/react" {
  interface Theme extends MuiTheme {
    deprecated: MuiTheme;
  }
}
