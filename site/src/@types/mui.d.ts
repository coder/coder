import type { PaletteColor, PaletteColorOptions } from "@mui/material/styles";
import type { NewTheme } from "theme/experimental";

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
