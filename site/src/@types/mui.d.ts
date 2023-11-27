import type {
  PaletteColor,
  PaletteColorOptions,
  Theme,
} from "@mui/material/styles";
import type { NewTheme } from "theme/experimental";

declare module "@mui/styles/defaultTheme" {
  interface DefaultTheme extends Theme {}
}

declare module "@mui/material/styles" {
  interface Theme {
    experimental: NewTheme;
  }

  interface Palette {
    neutral: PaletteColor;
  }

  interface PaletteOptions {
    neutral?: PaletteColorOptions;
  }
}

declare module "@mui/material/Button" {
  interface ButtonPropsColorOverrides {
    neutral: true;
  }

  interface ButtonPropsSizeOverrides {
    xlarge: true;
  }
}

declare module "@mui/system" {
  interface Theme {
    experimental: NewTheme;
  }
}
