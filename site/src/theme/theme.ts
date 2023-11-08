import colors from "./tailwind";

export interface CoderTheme {
  primary: Role; // page background, things which sit at the "root level"
  secondary: InteractiveRole; // sidebars, table headers, navigation
  tertiary: InteractiveRole; // buttons, inputs
  modal: Role; // modals/popovers/dropdowns

  roles: {
    danger: InteractiveRole; // delete, immutable parameters, stuff that sucks to fix
    error: Role; // something went wrong
    warning: Role; // something is amiss
    notice: Role; // like info, but actionable. "this is fine, but you may want to..."
    info: Role; // just sharing :)
    success: InteractiveRole; // yay!! it's working!!
    active: Role; // selected items, focused inputs,
  };
}

export interface Role {
  background: string;
  outline: string;
  fill: string;
  // contrastOutline?: string;
  text: string;
}

export interface InteractiveRole extends Role {
  disabled: Role;
  hover: Role;
}

export const darkReal = {
  primary: {
    background: colors.gray[950],
    outline: colors.gray[700],
    fill: "#f00",
    text: colors.white,
  },
  secondary: {
    background: colors.gray[900],
    outline: colors.gray[700],
    fill: "#f00",
    text: colors.white,
    disabled: {
      background: "#f00",
      outline: "#f00",
      fill: "#f00",
      text: colors.gray[200],
    },
    hover: {
      background: "#f00",
      outline: "#f00",
      fill: "#f00",
      text: colors.white,
    },
  },
  tertiary: {
    background: colors.gray[800],
    outline: colors.gray[700],
    fill: "#f00",
    text: colors.white,
    disabled: {
      background: "#f00",
      outline: "#f00",
      fill: "#f00",
      text: colors.gray[200],
    },
    hover: {
      background: "#f00",
      outline: "#f00",
      fill: "#f00",
      text: colors.white,
    },
  },
  modal: {
    background: "#f00",
    outline: "#f00",
    fill: "#f00",
    text: colors.white,
  },

  roles: {
    danger: {
      background: colors.orange[950],
      outline: colors.orange[500],
      fill: colors.orange[600],
      text: colors.orange[50],
      disabled: {
        background: colors.orange[950],
        outline: colors.orange[600],
        fill: colors.orange[800],
        text: colors.orange[200],
      },
      hover: {
        background: colors.orange[900],
        outline: colors.orange[500],
        fill: colors.orange[500],
        text: colors.orange[50],
      },
    },
    error: {
      background: colors.red[950],
      outline: colors.red[500],
      fill: colors.red[600],
      text: colors.red[50],
    },
    warning: {
      background: colors.amber[950],
      outline: colors.amber[300],
      fill: "#f00",
      text: colors.amber[50],
    },
    notice: {
      background: colors.yellow[950],
      outline: colors.yellow[200],
      fill: "#f00",
      text: colors.yellow[50],
    },
    info: {
      background: colors.blue[950],
      outline: colors.blue[400],
      fill: "#f00",
      text: colors.blue[50],
    },
    success: {
      background: colors.green[950],
      outline: colors.green[500],
      fill: colors.green[600],
      text: colors.green[50],
      disabled: {
        background: colors.green[950],
        outline: colors.green[600],
        fill: colors.green[800],
        text: colors.green[200],
      },
      hover: {
        background: colors.green[900],
        outline: colors.green[500],
        fill: colors.green[500],
        text: colors.green[50],
      },
    },
    active: {
      background: colors.sky[950],
      outline: colors.sky[500],
      fill: "#f00",
      text: colors.sky[50],
    },
  },
} satisfies CoderTheme;

export const dark = {
  primary: {
    background: colors.gray[300],
    outline: colors.gray[400],
    fill: "#f00",
    text: "#000",
  },
  secondary: {
    background: colors.gray[200],
    outline: colors.gray[400],
    fill: "#f00",
    text: "#000",
    disabled: {
      background: "#f00",
      outline: "#f00",
      fill: "#f00",
      text: colors.gray[800],
    },
    hover: {
      background: "#f00",
      outline: "#f00",
      fill: "#f00",
      text: "#000",
  typography: {
    fontFamily: BODY_FONT_FAMILY,

    body1: {
      fontSize: "1rem" /* 16px at default scaling */,
      lineHeight: "1.5rem" /* 24px at default scaling */,
    },

    body2: {
      fontSize: "0.875rem" /* 14px at default scaling */,
      lineHeight: "1.25rem" /* 20px at default scaling */,
    },
  },
  tertiary: {
    background: colors.gray[100],
    outline: colors.gray[400],
    fill: "#f00",
    text: "#000",
    disabled: {
      background: "#f00",
      outline: "#f00",
      fill: "#f00",
      text: colors.gray[800],
    },
    hover: {
      background: "#f00",
      outline: "#f00",
      fill: "#f00",
      text: colors.white,
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
        outlined: {
          ":hover": {
            border: `1px solid ${colors.gray[10]}`,
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
          ">button:hover+button": {
            // The !important is unfortunate, but necessary for the border.
            borderLeftColor: `${colors.gray[10]} !important`,
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
            borderColor: colors.gray[12],
          },
          // The default outlined input color is white, which seemed jarring.
          "&:hover:not(.Mui-error):not(.Mui-focused) .MuiOutlinedInput-notchedOutline":
            {
              borderColor: colors.gray[10],
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
  modal: {
    background: colors.gray[50],
    outline: "#f00",
    fill: "#f00",
    text: colors.white,
  },

  roles: {
    danger: {
      background: colors.orange[50],
      outline: colors.orange[600],
      fill: colors.orange[400],
      text: colors.orange[800],
      disabled: {
        background: colors.orange[50],
        outline: colors.orange[600],
        fill: colors.orange[400],
        text: colors.orange[700],
      },
      hover: {
        background: colors.orange[100],
        outline: colors.orange[500],
        fill: colors.orange[500],
        text: colors.orange[800],
      },
    },
    error: {
      background: colors.red[50],
      outline: colors.red[600],
      fill: colors.red[500],
      text: colors.red[800],
    },
    warning: {
      background: colors.amber[50],
      outline: colors.amber[600],
      fill: "#f00",
      text: colors.amber[800],
    },
    notice: {
      background: colors.yellow[50],
      outline: colors.yellow[700],
      fill: "#f00",
      text: colors.yellow[800],
    },
    info: {
      background: colors.blue[50],
      outline: colors.blue[600],
      fill: colors.blue[500],
      text: colors.blue[800],
    },
    success: {
      background: colors.green[50],
      outline: colors.green[500],
      fill: colors.green[600],
      text: colors.green[800],
      disabled: {
        background: colors.green[50],
        outline: colors.green[600],
        fill: colors.green[800],
        text: colors.green[800],
      },
      hover: {
        background: colors.green[100],
        outline: colors.green[500],
        fill: colors.green[500],
        text: colors.green[800],
      },
    },
    active: {
      background: colors.sky[50],
      outline: colors.sky[400],
      fill: "#f00",
      text: colors.sky[800],
    },
  },
} satisfies CoderTheme;
