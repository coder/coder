import { createTheme, type ThemeOptions } from "@mui/material/styles";
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

export const light = createTheme({
  palette: {
    mode: "light",
    primary: {
      light: colors.blue[6],
      main: coderTheme.secondary.fill,
      dark: coderTheme.secondary.background,
      contrastText: colors.blue[1],
    },
    secondary: {
      main: coderTheme.tertiary.fill,
      contrastText: colors.gray[4],
      dark: coderTheme.tertiary.background, // "#f00", // colors.indigo[1], //
    },
    background: {
      default: coderTheme.primary.background,
      paper: coderTheme.secondary.background,
      paperLight: coderTheme.tertiary.background,
      // paperLight: coderTheme.secondary.background,
      // paper: coderTheme.tertiary.background,
    },
    text: {
      primary: coderTheme.secondary.text,
      secondary: coderTheme.secondary.text,
      disabled: coderTheme.secondary.disabled.text,
    },
    divider: coderTheme.primary.outline,
    warning: {
      dark: coderTheme.roles.warning.outline,
      main: coderTheme.roles.warning.fill,
      light: coderTheme.roles.warning.background,
    },
    success: {
      dark: coderTheme.roles.success.outline,
      main: coderTheme.roles.success.fill,
      light: coderTheme.roles.success.background,
    },
    info: {
      dark: coderTheme.roles.info.outline,
      main: coderTheme.roles.info.fill,
      light: coderTheme.roles.info.background,
    },
    error: {
      dark: coderTheme.roles.error.outline,
      main: coderTheme.roles.error.fill,
      light: coderTheme.roles.error.background,
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

dark = createTheme(light, {
  components: {
    MuiCssBaseline: {
      styleOverrides: `
        html, body, #root, #storybook-root {
          height: 100%;
        }

        input:-webkit-autofill,
        input:-webkit-autofill:hover,
        input:-webkit-autofill:focus,
        input:-webkit-autofill:active  {
          -webkit-box-shadow: 0 0 0 100px ${dark.palette.background.default} inset !important;
        }

        ::placeholder {
          color: ${dark.palette.text.disabled};
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
          backgroundColor: coderTheme.tertiary.background,
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
        root: ({ theme }) => ({
          textTransform: "none",
          letterSpacing: "normal",
          fontWeight: 500,
          height: BUTTON_MD_HEIGHT,
          padding: theme.spacing(1, 2),
          borderRadius: "6px",
          fontSize: 14,

          whiteSpace: "nowrap",
          ":focus-visible": {
            outline: `2px solid ${theme.palette.primary.main}`,
          },

          "& .MuiLoadingButton-loadingIndicator": {
            width: 14,
            height: 14,
          },

          "& .MuiLoadingButton-loadingIndicator .MuiCircularProgress-root": {
            width: "inherit !important",
            height: "inherit !important",
          },
        }),
        sizeSmall: {
          height: BUTTON_SM_HEIGHT,
        },
        sizeLarge: {
          height: BUTTON_LG_HEIGHT,
        },
        outlined: {
          ":hover": {
            border: `1px solid ${colors.gray[9]}`,
          },
        },
        outlinedNeutral: {
          borderColor: colors.gray[12],

          "&.Mui-disabled": {
            borderColor: colors.gray[13],
            color: colors.gray[11],

            "& > .MuiLoadingButton-loadingIndicator": {
              color: colors.gray[11],
            },
          },
        },
        containedNeutral: {
          borderColor: colors.gray[12],
          backgroundColor: colors.gray[13],
          "&:hover": {
            backgroundColor: colors.gray[12],
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
          "> button:hover + button": {
            // The !important is unfortunate, but necessary for the border.
            borderLeftColor: `${colors.gray[9]} !important`,
          },
        },
      },
    },
    MuiTableContainer: {
      styleOverrides: {
        root: {
          borderRadius,
          border: `1px solid ${dark.palette.divider}`,
        },
      },
    },
    MuiTable: {
      styleOverrides: {
        root: ({ theme }) => ({
          borderCollapse: "unset",
          border: "none",
          background: coderTheme.tertiary.background,
          boxShadow: `0 0 0 1px ${dark.palette.background.default} inset`,
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
          color: dark.palette.text.secondary,
          fontWeight: 600,
          background: dark.palette.background.paperLight,
        },
        root: {
          fontSize: 16,
          background: dark.palette.background.paper,
          borderBottom: `1px solid ${dark.palette.divider}`,
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
          border: `1px solid ${dark.palette.divider}`,
          backgroundImage: "none",
        },
      },
    },
    MuiSkeleton: {
      styleOverrides: {
        root: {
          backgroundColor: dark.palette.divider,
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
          backgroundColor: colors.gray[12],
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
            borderColor: colors.gray[12],
          },
          // The default outlined input color is white, which seemed jarring.
          "&:hover:not(.Mui-error):not(.Mui-focused) .MuiOutlinedInput-notchedOutline":
            {
              borderColor: colors.gray[9],
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
          "&.Mui-disabled": {
            color: colors.gray[11],
          },
        },
      },
    },
    MuiSwitch: {
      defaultProps: {
        color: "primary",
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
          background: coderTheme.modal.background,
        },
      },
    },
    MuiAlert: {
      defaultProps: {
        variant: "outlined",
      },
      styleOverrides: {
        root: ({ theme, ownerState }) => ({
          backgroundColor:
            coderTheme.roles[ownerState.severity ?? "success"]?.background,
          color: coderTheme.roles[ownerState.severity ?? "success"]?.text,
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
  },
} as ThemeOptions);
