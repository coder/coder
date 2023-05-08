import { colors } from "./colors"
import { ThemeOptions, createTheme, Theme } from "@mui/material/styles"
import { BODY_FONT_FAMILY, borderRadius } from "./constants"

// MUI does not have aligned heights for buttons and inputs so we have to "hack" it a little bit
const INPUT_HEIGHT = 46
const BUTTON_LG_HEIGHT = 46

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
      main: colors.orange[11],
      dark: colors.orange[15],
    },
    success: {
      main: colors.green[11],
      dark: colors.green[15],
    },
    info: {
      main: colors.blue[11],
      dark: colors.blue[15],
      contrastText: colors.gray[4],
    },
    error: {
      main: colors.red[5],
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
    fontSize: 16,
    fontWeightLight: 300,
    fontWeightRegular: 400,
    fontWeightMedium: 500,
    h1: {
      fontSize: 72,
      fontWeight: 400,
      letterSpacing: -2,
    },
    h2: {
      fontSize: 64,
      letterSpacing: -2,
      fontWeight: 400,
    },
    h3: {
      fontSize: 32,
      letterSpacing: -0.3,
      fontWeight: 400,
    },
    h4: {
      fontSize: 24,
      letterSpacing: -0.3,
      fontWeight: 400,
    },
    h5: {
      fontSize: 20,
      letterSpacing: -0.3,
      fontWeight: 400,
    },
    h6: {
      fontSize: 16,
      fontWeight: 600,
    },
    body1: {
      fontSize: 16,
      lineHeight: "24px",
    },
    body2: {
      fontSize: 14,
      lineHeight: "20px",
    },
    subtitle1: {
      fontSize: 18,
      lineHeight: "28px",
    },
    subtitle2: {
      fontSize: 16,
      lineHeight: "24px",
    },
    caption: {
      fontSize: 12,
      lineHeight: "16px",
    },
    overline: {
      fontSize: 12,
      fontWeight: 500,
      lineHeight: "16px",
      letterSpacing: 1.5,
      textTransform: "uppercase",
    },
    button: {
      fontSize: 13,
      fontWeight: 600,
      textTransform: "uppercase",
      letterSpacing: 1.5,
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
        input:-webkit-autofill,
        input:-webkit-autofill:hover,
        input:-webkit-autofill:focus,
        input:-webkit-autofill:active  {
          -webkit-box-shadow: 0 0 0 100px ${dark.palette.background.default} inset !important;
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
        root: ({ ownerState }) => {
          let height: number | undefined = undefined

          if (ownerState.size === "large") {
            height = BUTTON_LG_HEIGHT
          }

          return {
            height,
            textTransform: "none",
            letterSpacing: "normal",
            fontWeight: 500,
          }
        },
      },
    },
    MuiIconButton: {
      styleOverrides: {
        sizeSmall: {
          "& .MuiSvgIcon-root": {
            width: 20,
            height: 20,
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
        root: {
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
        },
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
          "&:not(:only-child):first-child, &:not(:only-child):first-child > a":
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
      styleOverrides: {
        root: {
          color: dark.palette.primary.light,
        },
      },
    },
    MuiPaper: {
      defaultProps: {
        elevation: 0,
      },
      styleOverrides: {
        root: {
          borderRadius,
          border: `1px solid ${dark.palette.divider}`,
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
        // Disable the behavior of placing the select at the selected option
        getContentAnchorEl: null,
      },
      styleOverrides: {
        paper: {
          marginTop: 8,
          borderRadius: 4,
          padding: "4px 0",
          minWidth: 120,
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
        root: ({ ownerState }) => {
          let height: number | undefined = undefined

          if (ownerState.size === "medium") {
            height = INPUT_HEIGHT
          }

          return {
            height,
          }
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
  },
} as ThemeOptions)
