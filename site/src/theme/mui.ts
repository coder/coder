/* eslint-disable @typescript-eslint/no-explicit-any
-- we need to hack around the MUI types a little */
import { type ThemeOptions } from "@mui/material/styles";
// eslint-disable-next-line no-restricted-imports -- we use the classes for customization
import { alertClasses } from "@mui/material/Alert";
import {
  BODY_FONT_FAMILY,
  borderRadius,
  BUTTON_LG_HEIGHT,
  BUTTON_MD_HEIGHT,
  BUTTON_SM_HEIGHT,
  BUTTON_XL_HEIGHT,
} from "./constants";
import tw from "./tailwindColors";

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

export const components = {
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
      root: {
        width: 36,
        height: 36,
        fontSize: 18,

        "& .MuiSvgIcon-root": {
          width: "50%",
        },
      },
      colorDefault: ({ theme }) => ({
        backgroundColor: theme.palette.primary.light,
      }),
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
        padding: "8px 16px",
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
      ["sizeXlarge" as any]: {
        height: BUTTON_XL_HEIGHT,

        // With higher size we need to increase icon spacing.
        "& .MuiButton-startIcon": {
          marginRight: 12,
        },
        "& .MuiButton-endIcon": {
          marginLeft: 12,
        },
      },
      outlined: ({ theme }) => ({
        ":hover": {
          border: `1px solid ${theme.palette.secondary.main}`,
        },
      }),
      ["outlinedNeutral" as any]: {
        borderColor: tw.zinc[600],

        "&.Mui-disabled": {
          borderColor: tw.zinc[700],
          color: tw.zinc[500],

          "& > .MuiLoadingButton-loadingIndicator": {
            color: tw.zinc[500],
          },
        },
      },
      ["containedNeutral" as any]: {
        backgroundColor: tw.zinc[800],

        "&:hover": {
          backgroundColor: tw.zinc[700],
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
    },
  },
  MuiButtonGroup: {
    styleOverrides: {
      root: ({ theme }) => ({
        ">button:hover+button": {
          // The !important is unfortunate, but necessary for the border.
          borderLeftColor: `${theme.palette.secondary.main} !important`,
        },
      }),
    },
  },
  ["MuiLoadingButton" as any]: {
    defaultProps: {
      variant: "outlined",
      color: "neutral",
    },
  },
  MuiTableContainer: {
    styleOverrides: {
      root: ({ theme }) => ({
        borderRadius,
        border: `1px solid ${theme.palette.divider}`,
      }),
    },
  },
  MuiTable: {
    styleOverrides: {
      root: ({ theme }) => ({
        borderCollapse: "unset",
        border: "none",
        boxShadow: `0 0 0 1px ${theme.palette.background.default} inset`,
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
      head: ({ theme }) => ({
        fontSize: 14,
        color: theme.palette.text.secondary,
        fontWeight: 500,
        background: theme.palette.background.paper,
        borderWidth: 2,
      }),
      root: ({ theme }) => ({
        fontSize: 14,
        background: theme.palette.background.paper,
        borderBottom: `1px solid ${theme.palette.divider}`,
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
      }),
    },
  },
  MuiTableRow: {
    styleOverrides: {
      root: ({ theme }) => ({
        "&:last-child .MuiTableCell-body": {
          borderBottom: 0,
        },

        "&.MuiTableRow-hover:hover": {
          backgroundColor: theme.palette.background.paper,
        },
      }),
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
      root: ({ theme }) => ({
        border: `1px solid ${theme.palette.divider}`,
        backgroundImage: "none",
      }),
    },
  },
  MuiSkeleton: {
    styleOverrides: {
      root: ({ theme }) => ({
        backgroundColor: theme.palette.divider,
      }),
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
        backgroundColor: tw.zinc[600],
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
      ["colorPrimary" as any]: {
        // Same as button
        "& .MuiOutlinedInput-notchedOutline": {
          borderColor: tw.zinc[600],
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
          boxShadow: `0 0 0 2px ${tw.blue[400]}`,
        },

        "&.Mui-disabled": {
          color: tw.zinc[500],
        },

        "& .MuiSvgIcon-fontSizeXsmall": {
          fontSize: "1rem",
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
          boxShadow: `0 0 0 3px ${tw.blue[400]}`,
        },
      },
    },
  },
  MuiAutocomplete: {
    styleOverrides: {
      root: {
        // Not sure why but since the input has padding we don't need it here
        "& .MuiInputBase-root": {
          padding: "0px 0px 0px 8px",
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
      tooltip: ({ theme }) => ({
        lineHeight: "150%",
        borderRadius: 4,
        background: theme.palette.divider,
        padding: "8px 16px",
      }),
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
      outlinedWarning: ({ theme }) => ({
        [`& .${alertClasses.icon}`]: {
          color: theme.palette.warning.light,
        },
      }),
      outlinedInfo: ({ theme }) => ({
        [`& .${alertClasses.icon}`]: {
          color: theme.palette.primary.light,
        },
      }),
      outlinedError: ({ theme }) => ({
        [`& .${alertClasses.icon}`]: {
          color: theme.palette.error.light,
        },
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

  MuiIconButton: {
    styleOverrides: {
      root: {
        "&.Mui-focusVisible": {
          boxShadow: `0 0 0 2px ${tw.blue[400]}`,
        },
      },
    },
  },
} satisfies ThemeOptions["components"];
