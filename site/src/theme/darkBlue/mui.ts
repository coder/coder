import { createTheme } from "@mui/material/styles";
import { BODY_FONT_FAMILY, borderRadius } from "../constants";
import tw from "../tailwindColors";
import { components } from "../mui";

const muiTheme = createTheme({
  palette: {
    mode: "dark",
    primary: {
      main: tw.sky[500],
      contrastText: tw.sky[50],
      light: tw.sky[300],
      dark: tw.sky[400],
    },
    secondary: {
      main: tw.gray[500],
      contrastText: tw.gray[200],
      dark: tw.gray[400],
    },
    background: {
      default: tw.gray[900],
      paper: tw.gray[900],
    },
    text: {
      primary: tw.gray[50],
      secondary: tw.gray[300],
      disabled: tw.gray[400],
    },
    divider: tw.gray[700],
    warning: {
      light: tw.amber[500],
      main: tw.amber[800],
      dark: tw.amber[950],
    },
    success: {
      main: tw.green[500],
      dark: tw.green[600],
    },
    info: {
      light: tw.blue[400],
      main: tw.blue[600],
      dark: tw.blue[950],
      contrastText: tw.gray[200],
    },
    error: {
      light: tw.red[400],
      main: tw.red[500],
      dark: tw.red[950],
      contrastText: tw.gray[200],
    },
    action: {
      hover: tw.gray[800],
    },
    neutral: {
      main: tw.gray[50],
    },
    dots: tw.gray[500],
  },
  typography: {
    fontFamily: BODY_FONT_FAMILY,

    body1: {
      fontSize: "1rem" /* 16px at default scaling */,
      lineHeight: "160%",
    },

    body2: {
      fontSize: "0.875rem" /* 14px at default scaling */,
      lineHeight: "160%",
    },
  },
  shape: {
    borderRadius,
  },
  components,
});

export default muiTheme;
