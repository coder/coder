import { createTheme } from "@mui/material/styles";
import { colors } from "./colors";
import { BODY_FONT_FAMILY, borderRadius } from "./constants";
import tw from "./tailwind";
import { dark as coderTheme } from "./theme";

// MUI does not have aligned heights for buttons and inputs so we have to "hack" it a little bit
export const BUTTON_LG_HEIGHT = 40;
export const BUTTON_MD_HEIGHT = 36;
export const BUTTON_SM_HEIGHT = 32;

export type PaletteIndex =
  | "primary"
  | "secondary"
  | "background"
  | "text"
  | "error"
  | "warning"
  | "info"
  | "success"
  | "action"
  | "neutral";

export let dark = createTheme({
  palette: {
    mode: "dark",
    primary: {
      light: colors.blue[6],
      main: colors.blue[7],
      dark: colors.blue[9],
      contrastText: colors.blue[1],
    },
    secondary: {
      main: colors.gray[11],
      contrastText: colors.gray[4],
      dark: tw.sky[800], // "#f00", // colors.indigo[1], //
    },
    background: {
      default: coderTheme.primary.background,
      paper: coderTheme.secondary.background,
      paperLight: coderTheme.tertiary.background,
    },
    text: {
      primary: coderTheme.secondary.text,
      secondary: coderTheme.secondary.text,
      disabled: coderTheme.secondary.disabled.text,
    },
    divider: coderTheme.primary.outline,
    warning: {
      light: coderTheme.roles.warning.outline,
      main: coderTheme.roles.warning.fill,
      dark: coderTheme.roles.warning.background,
      // light: colors.orange[7],
      // main: colors.orange[9],
      // dark: colors.orange[15],
    },
    success: {
      light: coderTheme.roles.success.outline,
      main: coderTheme.roles.success.fill,
      dark: coderTheme.roles.success.background,
      // main: colors.green[11],
      // dark: colors.green[15],
    },
    info: {
      light: coderTheme.roles.info.outline,
      main: coderTheme.roles.info.fill,
      dark: coderTheme.roles.info.background,
      // light: colors.blue[7],
      // main: colors.blue[9],
      // dark: colors.blue[14],
      // contrastText: colors.gray[4],
    },
    error: {
      light: coderTheme.roles.error.outline,
      main: coderTheme.roles.error.fill,
      dark: coderTheme.roles.error.background,
      // light: colors.red[6],
      // main: colors.red[8],
      // dark: colors.red[15],
      // contrastText: colors.gray[4],
    },
    action: {
      hover: colors.gray[14],
    },
    neutral: {
      main: colors.gray[1],
    },
  },
  typography: {
    fontFamily: BODY_FONT_FAMILY,
    body1: {
      fontSize: 16,
      lineHeight: "24px",
    },
    body2: {
      fontSize: 14,
      lineHeight: "20px",
    },
  },
  shape: {
    borderRadius,
  },
});
