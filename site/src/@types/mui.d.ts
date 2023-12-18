import type { PaletteColor, PaletteColorOptions } from "@mui/material/styles";

declare module "@mui/material/styles" {
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
