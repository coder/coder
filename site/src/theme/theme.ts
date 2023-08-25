import { colors } from "./colors"
import { ThemeOptions, createTheme, Theme } from "@mui/material/styles"
import { BODY_FONT_FAMILY, borderRadius } from "./constants"

// MUI does not have aligned heights for buttons and inputs so we have to "hack" it a little bit
export const BUTTON_LG_HEIGHT = 40
export const BUTTON_MD_HEIGHT = 36
export const BUTTON_SM_HEIGHT = 32

export type PaletteIndex = keyof Theme["palette"]
export type PaletteStatusIndex = Extract<
  PaletteIndex,
  "error" | "warning" | "info" | "success"
>

export let dark = createTheme({
  palette: {
    mode: "dark",
    primary: {
      main: colors.blue[7],
      contrastText: colors.blue[1],
      light: colors.blue[6],
      dark: colors.blue[9],
    },
    secondary: {
      main: colors.gray[11],
      contrastText: colors.gray[4],
      dark: colors.indigo[7],
    },
    background: {
      default: colors.gray[17],
      paper: colors.gray[16],
      paperLight: colors.gray[15],
    },
    text: {
      primary: colors.gray[1],
      secondary: colors.gray[5],
      disabled: colors.gray[7],
    },
    divider: colors.gray[13],
    warning: {
      light: colors.orange[7],
      main: colors.orange[9],
      dark: colors.orange[15],
    },
    success: {
      main: colors.green[11],
      dark: colors.green[15],
    },
    info: {
      light: colors.blue[7],
      main: colors.blue[9],
      dark: colors.blue[15],
      contrastText: colors.gray[4],
    },
    error: {
      light: colors.red[6],
      main: colors.red[8],
      dark: colors.red[15],
      contrastText: colors.gray[4],
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
})

dark = createTheme(dark, {
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
          backgroundColor: colors.gray[6],
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
          background: dark.palette.background.paper,
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
              borderColor: colors.gray[7],
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
          background: dark.palette.divider,
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
} as ThemeOptions)
