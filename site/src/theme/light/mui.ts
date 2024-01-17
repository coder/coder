/* eslint-disable @typescript-eslint/no-explicit-any
-- we need to hack around the MUI types a little */
import { createTheme } from "@mui/material/styles";
import { BODY_FONT_FAMILY, borderRadius } from "../constants";
import { components } from "../mui";
import tw from "../tailwindColors";

const muiTheme = createTheme({
  palette: {
    mode: "light",
    primary: {
      main: tw.sky[600],
      contrastText: tw.sky[50],
      light: tw.sky[400],
      dark: tw.sky[500],
    },
    secondary: {
      main: tw.zinc[500],
      contrastText: tw.zinc[800],
      dark: tw.zinc[600],
    },
    background: {
      default: tw.zinc[50],
      paper: tw.zinc[100],
    },
    text: {
      primary: tw.zinc[950],
      secondary: tw.zinc[700],
      disabled: tw.zinc[600],
    },
    divider: tw.zinc[200],
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
      contrastText: tw.zinc[200],
    },
    error: {
      light: tw.red[400],
      main: tw.red[500],
      dark: tw.red[950],
      contrastText: tw.zinc[800],
    },
    action: {
      hover: tw.zinc[100],
    },
    neutral: {
      main: tw.zinc[950],
    },
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
  components: {
    ...components,
    MuiCssBaseline: {
      styleOverrides: (theme) => `
        html, body, #root, #storybook-root {
          height: 100%;
        }

        button, input {
          font-family: ${BODY_FONT_FAMILY};
        }

        input:-webkit-autofill,
        input:-webkit-autofill:hover,
        input:-webkit-autofill:focus,
        input:-webkit-autofill:active  {
          -webkit-box-shadow: 0 0 0 100px ${theme.palette.background.default} inset !important;
        }

        ::placeholder {
          color: ${theme.palette.text.disabled};
        }
      `,
    },
    MuiAvatar: {
      styleOverrides: {
        root: components.MuiAvatar.styleOverrides.root,
        colorDefault: {
          backgroundColor: tw.zinc[700],
        },
      },
    },
    MuiButton: {
      ...components.MuiButton,
      styleOverrides: {
        ...components.MuiButton.styleOverrides,
        outlined: ({ theme }) => ({
          boxShadow: "0 1px 4px #0001",
          ":hover": {
            boxShadow: "0 1px 4px #0001",
            border: `1px solid ${theme.palette.secondary.main}`,
          },
          "&.Mui-disabled": {
            boxShadow: "none !important",
          },
        }),
        ["outlinedNeutral" as any]: {
          borderColor: tw.zinc[300],

          "&.Mui-disabled": {
            borderColor: tw.zinc[200],
            color: tw.zinc[500],

            "& > .MuiLoadingButton-loadingIndicator": {
              color: tw.zinc[500],
            },
          },
        },
        contained: {
          boxShadow: "0 1px 4px #0001",
          "&.Mui-disabled": {
            boxShadow: "none !important",
          },
          ":hover": {
            boxShadow: "0 1px 4px #0001",
          },
        },
        ["containedNeutral" as any]: {
          backgroundColor: tw.zinc[100],
          border: `1px solid ${tw.zinc[200]}`,

          "&.Mui-disabled": {
            backgroundColor: tw.zinc[50],
            border: `1px solid ${tw.zinc[100]}`,
          },

          "&:hover": {
            backgroundColor: tw.zinc[200],
            border: `1px solid ${tw.zinc[300]}`,
          },
        },
      },
    },
    MuiButtonGroup: {
      styleOverrides: {
        root: {
          ">button:hover+button": {
            // The !important is unfortunate, but necessary for the border.
            borderLeftColor: `${tw.zinc[300]} !important`,
          },
        },
      },
    },
    MuiChip: {
      styleOverrides: {
        root: {
          backgroundColor: tw.zinc[400],
        },
      },
    },
    MuiInputBase: {
      ...components.MuiInputBase,
      styleOverrides: {
        ...components.MuiInputBase.styleOverrides,
        ["colorPrimary" as any]: {
          // Same as button
          "& .MuiOutlinedInput-notchedOutline": {
            borderColor: tw.zinc[300],
          },
          // The default outlined input color is white, which seemed jarring.
          "&:hover:not(.Mui-error):not(.Mui-focused) .MuiOutlinedInput-notchedOutline":
            {
              borderColor: tw.zinc[500],
            },
        },
      },
    },
    MuiCheckbox: {
      styleOverrides: {
        root: {
          /**
           * Adds focus styling to checkboxes (which doesn't exist normally, for
           * some reason?).
           *
           * The checkbox component is a root span with a checkbox input inside
           * it. MUI does not allow you to use selectors like (& input) to
           * target the inner checkbox (even though you can use & td to style
           * tables). Tried every combination of selector possible (including
           * lots of !important), and the main issue seems to be that the
           * styling just never gets processed for it to get injected into the
           * CSSOM.
           *
           * Had to settle for adding styling to the span itself (which does
           * make the styling more obvious, even if there's not much room for
           * customization).
           */
          "&.Mui-focusVisible": {
            boxShadow: `0 0 0 2px ${tw.blue[600]}`,
          },

          "&.Mui-disabled": {
            color: tw.zinc[500],
          },
        },
      },
    },
    MuiSwitch: {
      ...components.MuiSwitch,
      styleOverrides: {
        root: {
          ".Mui-focusVisible .MuiSwitch-thumb": {
            // Had to thicken outline to make sure that the focus color didn't
            // bleed into the thumb and was still easily-visible
            boxShadow: `0 0 0 3px ${tw.blue[600]}`,
          },
        },
      },
    },
    MuiTooltip: {
      styleOverrides: {
        tooltip: ({ theme }) => ({
          lineHeight: "150%",
          borderRadius: 4,
          background: theme.palette.background.paper,
          color: theme.palette.secondary.contrastText,
          border: `1px solid ${theme.palette.divider}`,
          padding: "8px 16px",
          boxShadow: "0 1px 4px #0001",
        }),
      },
    },
    MuiIconButton: {
      styleOverrides: {
        root: {
          "&.Mui-focusVisible": {
            boxShadow: `0 0 0 2px ${tw.blue[600]}`,
          },
        },
      },
    },
  },
});

export default muiTheme;
