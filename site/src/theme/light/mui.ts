// eslint-disable-next-line no-restricted-imports -- We need MUI here
import { alertClasses } from "@mui/material/Alert";
import { createTheme, type ThemeOptions } from "@mui/material/styles";
import {
  BODY_FONT_FAMILY,
  borderRadius,
  BUTTON_LG_HEIGHT,
  BUTTON_MD_HEIGHT,
  BUTTON_SM_HEIGHT,
  BUTTON_XL_HEIGHT,
} from "../constants";
import tw from "../tailwindColors";

let muiTheme = createTheme({
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
});

muiTheme = createTheme(muiTheme, {
  components: {
    MuiCssBaseline: {
      styleOverrides: `
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
          -webkit-box-shadow: 0 0 0 100px ${muiTheme.palette.background.default} inset !important;
        }

        ::placeholder {
          color: ${muiTheme.palette.text.disabled};
        }
      `,
    },
    MuiAvatar: {
      styleOverrides: {
        root: {
          width: 36,
          height: 36,
          fontSize: 18,

          "& .MuiSvgIcon-root": {
            width: "50%",
          },
        },
        colorDefault: {
          backgroundColor: tw.zinc[700],
        },
      },
    },
    // Button styles are based on
    // https://tailwindui.com/components/application-ui/elements/buttons
    MuiButtonBase: {
      defaultProps: {
        disableRipple: true,
      },
    },
    MuiButton: {
      defaultProps: {
        variant: "outlined",
        color: "neutral",
      },
      styleOverrides: {
        root: {
          textTransform: "none",
          letterSpacing: "normal",
          fontWeight: 500,
          height: BUTTON_MD_HEIGHT,
          padding: "8px 16px",
          borderRadius: "6px",
          fontSize: 14,

          whiteSpace: "nowrap",
          ":focus-visible": {
            outline: `2px solid ${muiTheme.palette.primary.main}`,
          },

          "& .MuiLoadingButton-loadingIndicator": {
            width: 14,
            height: 14,
          },

          "& .MuiLoadingButton-loadingIndicator .MuiCircularProgress-root": {
            width: "inherit !important",
            height: "inherit !important",
          },
        },
        sizeSmall: {
          height: BUTTON_SM_HEIGHT,
        },
        sizeLarge: {
          height: BUTTON_LG_HEIGHT,
        },
        sizeXlarge: {
          height: BUTTON_XL_HEIGHT,
        },
        outlined: {
          boxShadow: "0 1px 4px #0001",
          ":hover": {
            boxShadow: "0 1px 4px #0001",
            border: `1px solid ${tw.zinc[500]}`,
          },
          "&.Mui-disabled": {
            boxShadow: "none !important",
          },
        },
        outlinedNeutral: {
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
        containedNeutral: {
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
        iconSizeMedium: {
          "& > .MuiSvgIcon-root": {
            fontSize: 14,
          },
        },
        iconSizeSmall: {
          "& > .MuiSvgIcon-root": {
            fontSize: 13,
          },
        },
        startIcon: {
          marginLeft: "-2px",
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
    MuiLoadingButton: {
      defaultProps: {
        variant: "outlined",
        color: "neutral",
      },
    },
    MuiTableContainer: {
      styleOverrides: {
        root: {
          borderRadius,
          border: `1px solid ${muiTheme.palette.divider}`,
        },
      },
    },
    MuiTable: {
      styleOverrides: {
        root: ({ theme }) => ({
          borderCollapse: "unset",
          border: "none",
          boxShadow: `0 0 0 1px ${muiTheme.palette.background.default} inset`,
          overflow: "hidden",

          "& td": {
            paddingTop: 16,
            paddingBottom: 16,
            background: "transparent",
          },

          [theme.breakpoints.down("md")]: {
            minWidth: 1000,
          },
        }),
      },
    },
    MuiTableCell: {
      styleOverrides: {
        head: {
          fontSize: 14,
          color: muiTheme.palette.text.secondary,
          fontWeight: 600,
          background: muiTheme.palette.background.paper,
        },
        root: {
          fontSize: 16,
          background: muiTheme.palette.background.paper,
          borderBottom: `1px solid ${muiTheme.palette.divider}`,
          padding: "12px 8px",
          // This targets the first+last td elements, and also the first+last elements
          // of a TableCellLink.
          "&:not(:only-child):first-of-type, &:not(:only-child):first-of-type > a":
            {
              paddingLeft: 32,
            },
          "&:not(:only-child):last-child, &:not(:only-child):last-child > a": {
            paddingRight: 32,
          },
        },
      },
    },
    MuiTableRow: {
      styleOverrides: {
        root: {
          "&:last-child .MuiTableCell-body": {
            borderBottom: 0,
          },
        },
      },
    },
    MuiLink: {
      defaultProps: {
        underline: "hover",
      },
    },
    MuiPaper: {
      defaultProps: {
        elevation: 0,
      },
      styleOverrides: {
        root: {
          border: `1px solid ${muiTheme.palette.divider}`,
          backgroundImage: "none",
        },
      },
    },
    MuiSkeleton: {
      styleOverrides: {
        root: {
          backgroundColor: muiTheme.palette.divider,
        },
      },
    },
    MuiLinearProgress: {
      styleOverrides: {
        root: {
          borderRadius: 999,
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
    MuiMenu: {
      defaultProps: {
        anchorOrigin: {
          vertical: "bottom",
          horizontal: "right",
        },
        transformOrigin: {
          vertical: "top",
          horizontal: "right",
        },
      },
      styleOverrides: {
        paper: {
          marginTop: 8,
          borderRadius: 4,
          padding: "4px 0",
          minWidth: 160,
        },
        root: {
          // It should be the same as the menu padding
          "& .MuiDivider-root": {
            marginTop: `4px !important`,
            marginBottom: `4px !important`,
          },
        },
      },
    },
    MuiMenuItem: {
      styleOverrides: {
        root: {
          gap: 12,

          "& .MuiSvgIcon-root": {
            fontSize: 20,
          },
        },
      },
    },
    MuiSnackbar: {
      styleOverrides: {
        anchorOriginBottomRight: {
          bottom: `${24 + 36}px !important`, // 36 is the bottom bar height
        },
      },
    },
    MuiSnackbarContent: {
      styleOverrides: {
        root: {
          borderRadius: "4px !important",
        },
      },
    },
    MuiTextField: {
      defaultProps: {
        InputLabelProps: {
          shrink: true,
        },
      },
    },
    MuiInputBase: {
      defaultProps: {
        color: "primary",
      },
      styleOverrides: {
        root: {
          height: BUTTON_LG_HEIGHT,
        },
        sizeSmall: {
          height: BUTTON_MD_HEIGHT,
          fontSize: 14,
        },
        multiline: {
          height: "auto",
        },
        colorPrimary: {
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
    MuiFormHelperText: {
      defaultProps: {
        sx: {
          marginLeft: 0,
          marginTop: 1,
        },
      },
    },
    MuiRadio: {
      defaultProps: {
        disableRipple: true,
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
      defaultProps: { color: "primary" },
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
    MuiAutocomplete: {
      styleOverrides: {
        root: {
          // Not sure why but since the input has padding we don't need it here
          "& .MuiInputBase-root": {
            padding: 0,
          },
        },
      },
    },
    MuiList: {
      defaultProps: {
        disablePadding: true,
      },
    },
    MuiTabs: {
      defaultProps: {
        textColor: "primary",
        indicatorColor: "primary",
      },
    },
    MuiTooltip: {
      styleOverrides: {
        tooltip: {
          lineHeight: "150%",
          borderRadius: 4,
          background: muiTheme.palette.background.paper,
          color: muiTheme.palette.secondary.contrastText,
          border: `1px solid ${muiTheme.palette.divider}`,
          padding: "8px 16px",
          boxShadow: "0 1px 4px #0001",
        },
      },
    },
    MuiAlert: {
      defaultProps: {
        variant: "outlined",
      },
      styleOverrides: {
        root: ({ theme }) => ({
          background: theme.palette.background.paper,
        }),
        action: {
          paddingTop: 2, // Idk why it is not aligned as expected
        },
        icon: {
          fontSize: 16,
          marginTop: "4px", // The size of text is 24 so (24 - 16)/2 = 4
        },
        message: ({ theme }) => ({
          color: theme.palette.text.primary,
        }),
        outlinedWarning: {
          [`& .${alertClasses.icon}`]: {
            color: muiTheme.palette.warning.light,
          },
        },
        outlinedInfo: {
          [`& .${alertClasses.icon}`]: {
            color: muiTheme.palette.primary.light,
          },
        },
        outlinedError: {
          [`& .${alertClasses.icon}`]: {
            color: muiTheme.palette.error.light,
          },
        },
      },
    },
    MuiAlertTitle: {
      styleOverrides: {
        root: {
          fontSize: "inherit",
          marginBottom: 0,
        },
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
} as ThemeOptions);

export default muiTheme;
