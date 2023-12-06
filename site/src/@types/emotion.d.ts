import type { Theme as CoderTheme } from "theme";

declare module "@emotion/react" {
  interface Theme extends CoderTheme {}
}
