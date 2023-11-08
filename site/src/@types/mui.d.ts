import type {
  PaletteColor,
  PaletteColorOptions,
  Theme,
} from "@mui/material/styles";

declare module "@mui/styles/defaultTheme" {
  /**
   * @deprecated
   */
  interface DefaultTheme extends Theme {}
}

declare module "@mui/material/styles" {
  /**
   * @deprecated
   */
  interface Theme {}

  interface TypeBackground {
    paperLight: string;
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
}
