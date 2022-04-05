import { darken, fade } from "@material-ui/core/styles/colorManipulator"
import { Breakpoints } from "@material-ui/core/styles/createBreakpoints"
import {
  BODY_FONT_FAMILY,
  borderRadius,
  buttonBorderWidth,
  emptyBoxShadow,
  lightButtonShadow,
  spacing,
} from "./constants"
import { CustomPalette } from "./palettes"
import { typography } from "./typography"

// eslint-disable-next-line @typescript-eslint/explicit-module-boundary-types
export const getOverrides = (palette: CustomPalette, breakpoints: Breakpoints) => {
  const containedButtonShadow = palette.type === "dark" ? emptyBoxShadow : lightButtonShadow
  return {
    MuiAvatar: {
      root: {
        width: 32,
        height: 32,
        fontSize: 24,
      },
    },
    MuiTable: {
      root: {
        backgroundColor: palette.background.paper,
        borderRadius: borderRadius * 2,
        boxShadow: `0 0 0 1px ${palette.action.hover}`,

        "&.is-inline": {
          background: "none",
          borderRadius: 0,
          boxShadow: "none",
        },
      },
    },
    MuiTableHead: {
      root: {
        "& .MuiTableCell-root": {
          ...typography.overline,
          paddingTop: spacing,
          paddingBottom: spacing,
          color: palette.text.secondary,
          backgroundColor: palette.background.default,

          ".MuiTable-root.is-inline &": {
            background: "none",
          },
        },
        "& .MuiTableRow-root:first-child .MuiTableCell-root:first-child": {
          borderTopLeftRadius: borderRadius * 2,

          ".MuiTable-root.is-inline &": {
            borderRadius: 0,
          },
        },
        "& .MuiTableRow-root:first-child .MuiTableCell-root:last-child": {
          borderTopRightRadius: borderRadius * 2,

          ".MuiTable-root.is-inline &": {
            borderRadius: 0,
          },
        },
      },
    },
    MuiTableSortLabel: {
      root: {
        lineHeight: "20px",

        "&$active": {
          "&& $icon": {
            color: palette.text.primary,
          },
        },
      },
      icon: {
        "&.MuiSvgIcon-root": {
          width: 18,
          height: 18,
          marginLeft: 2,
        },
      },
    },
    MuiTableCell: {
      root: {
        "&:first-child": {
          paddingLeft: spacing * 4,
        },
      },
    },
    MuiTablePagination: {
      input: {
        height: "auto",
        marginTop: 0,
        border: `1px solid ${palette.action.disabled}`,
      },

      select: {
        paddingTop: 6,
        fontSize: 14,
      },

      selectIcon: {
        right: -4,
      },
    },
    MuiList: {
      root: {
        boxSizing: "border-box",

        "& .MuiDivider-root": {
          opacity: 0.5,
        },
      },
    },
    MuiListItem: {
      root: {
        minHeight: 24,
        position: "relative",
        "&.active": {
          color: "white",
        },
      },
      gutters: {
        padding: spacing / 2,
        paddingLeft: spacing * 2,
        paddingRight: spacing * 2,
      },
    },
    MuiListItemAvatar: {
      root: {
        minWidth: 46,
      },
    },
    MuiListItemIcon: {
      root: {
        minWidth: 36,
        color: palette.text.primary,

        "& .MuiSvgIcon-root": {
          width: 20,
          height: 20,
        },
      },
    },
    MuiListSubheader: {
      root: {
        ...typography.overline,
        lineHeight: 2.5,
      },
      sticky: {
        backgroundColor: palette.background.paper,
      },
    },
    MuiMenu: {
      paper: {
        marginTop: spacing,
      },
    },
    MuiMenuItem: {
      root: {
        minHeight: 48,
        fontSize: 16,
        [breakpoints.up("sm")]: {
          minHeight: 48,
        },
      },
      gutters: {
        padding: spacing,
        paddingLeft: spacing * 2,
        paddingRight: spacing * 2,
      },
    },
    MuiListItemText: {
      primary: {
        fontWeight: 500,
      },
      secondary: {
        fontSize: 12,
      },
    },
    MuiOutlinedInput: {
      root: {
        fontSize: 14,
        "&$focused $notchedOutline": {
          borderWidth: 1,
        },
      },
      notchedOutline: {
        borderWidth: 1,
        borderColor: palette.action.disabled,
      },
    },
    MuiButton: {
      root: {
        minHeight: 40,
        paddingLeft: 20,
        paddingRight: 20,
        fontWeight: 500,

        "& .MuiSvgIcon-root": {
          verticalAlign: "middle",
        },
      },

      contained: {
        backgroundColor: palette.hero.button,
        color: palette.primary.contrastText,
        boxShadow: containedButtonShadow,
        "&:hover": {
          backgroundColor: darken(palette.hero.button, 0.25),
        },
        "&$disabled": {
          color: fade(palette.text.disabled, 0.5),
        },
      },
      containedPrimary: {
        boxShadow: containedButtonShadow,
      },
      containedSecondary: {
        boxShadow: containedButtonShadow,
      },

      outlined: {
        borderColor: palette.action.disabled,
        borderWidth: buttonBorderWidth,
        "&:hover": {
          color: palette.primary.main,
          borderColor: palette.primary.main,
          borderWidth: buttonBorderWidth,
        },
        "&$disabled": {
          color: fade(palette.text.disabled, 0.5),
        },
      },
      outlinedPrimary: {
        borderColor: palette.primary.main,
        borderWidth: buttonBorderWidth,
        "&:hover": {
          borderWidth: buttonBorderWidth,
        },
      },
      outlinedSecondary: {
        borderColor: palette.secondary.main,
        borderWidth: buttonBorderWidth,
        "&:hover": {
          color: palette.secondary.main,
          borderWidth: buttonBorderWidth,
        },
      },

      text: {
        "&$disabled": {
          color: fade(palette.text.disabled, 0.5),
        },
      },

      sizeSmall: {
        minHeight: 32,
        paddingLeft: 10,
        paddingRight: 10,
        letterSpacing: 1.2,
        fontSize: 13,

        "&.MuiButton-outlined": {
          borderWidth: 1,
        },
      },
      sizeLarge: {
        minHeight: 46,
        paddingLeft: spacing * 3,
        paddingRight: spacing * 3,
      },
    },
    MuiButtonGroup: {
      contained: {
        boxShadow: containedButtonShadow,
      },
    },
    MuiLink: {
      root: {
        fontWeight: 600,
      },
    },
    MuiInputBase: {
      root: {
        minHeight: 40,
        background: palette.background.paper,
        marginTop: "12px",
        borderRadius,

        "&$disabled": {
          background: palette.action.disabledBackground,
        },

        "&$focused .MuiSelect-icon": {
          color: palette.primary.light,
        },
      },

      input: {
        fontSize: 16,

        "&::placeholder": {
          color: palette.text.secondary,
          opacity: 1,
        },

        // See this thread https://stackoverflow.com/questions/69196525/how-to-change-the-color-of-the-calendar-icon-of-the-textfield-date-picker-in-ma
        "&::-webkit-calendar-picker-indicator": {
          filter:
            palette.type === "dark"
              ? "invert(95%) sepia(0%) saturate(1082%) hue-rotate(173deg) brightness(84%) contrast(80%);"
              : undefined,
        },
      },
    },
    MuiInputLabel: {
      shrink: {
        fontSize: 14,
        marginTop: "2px",
        left: "-12px",
        transform: "translate(16px, -6px) scale(0.8)",
        color: palette.text.primary,
      },
      outlined: {
        letterSpacing: "0.2px",
        lineHeight: "16px",
        fontFamily: BODY_FONT_FAMILY,

        "&$shrink": {
          transform: "translate(14px, -13px)",
        },
      },
    },
    MuiChip: {
      root: {
        fontSize: 16,
        fontWeight: 500,
        borderRadius: borderRadius,
        backgroundColor: fade(palette.text.secondary, 0.1),
        color: palette.text.secondary,
      },
      sizeSmall: {
        height: 20,
      },
      labelSmall: {
        fontSize: 12,
        paddingLeft: 6,
        paddingRight: 6,
      },
      colorPrimary: {
        backgroundColor: fade(palette.primary.main, 0.1),
        color: palette.primary.main,
      },
      colorSecondary: {
        backgroundColor: fade(palette.secondary.main, 0.15),
        color: palette.secondary.main,
      },
    },
    MuiSelect: {
      root: {
        // Matches MuiInputBase-input height
        lineHeight: "1.1875em",
        overflow: "hidden",
      },
      select: {
        "&:focus": {
          borderRadius,
        },
      },
      icon: {
        "&.MuiSvgIcon-root": {
          fontSize: 20,
          margin: "3px 6px 0 0",
        },
      },
      iconOutlined: {
        right: 0,
      },
      iconOpen: {
        transform: "none",
      },
    },

    MuiDialog: {
      paper: {
        borderRadius: 11,
      },
      paperWidthSm: {
        maxWidth: "530px",
      },
    },
    MuiDialogTitle: {
      root: {
        color: palette.text.primary,
        padding: `27px ${spacing * 6}px 0 ${spacing * 6}px`,
        fontSize: 32,
        fontWeight: 700,
        display: "flex",
        alignItems: "center",
        justifyContent: "space-between",
        background: palette.background.paper,
      },
    },
    MuiDialogContent: {
      root: {
        padding: `0 ${spacing * 6}px ${spacing * 6}px ${spacing * 6}px`,
      },
    },
    MuiDialogContentText: {
      root: {
        color: palette.action.active,
      },
    },
    MuiDialogActions: {
      root: {
        padding: 0,
      },
      spacing: {
        "& > :not(:first-child)": {
          marginLeft: 0,
        },
      },
    },
    MuiFormHelperText: {
      root: {
        fontSize: 12,
        lineHeight: "16px",
      },
      contained: {
        marginTop: 6,
        marginLeft: spacing * 0.5,
      },
    },
    MuiBackdrop: {
      root: {
        backgroundColor: fade("#242424", 0.6),
      },
    },
    MuiInputAdornment: {
      root: {
        "& .MuiSvgIcon-root": {
          color: palette.text.secondary,
        },
      },
      positionStart: {
        marginRight: 12,
        marginLeft: 6,
      },
    },
    MuiCheckbox: {
      root: {
        "& .MuiSvgIcon-root": {
          width: 16,
          height: 16,
          margin: 1,
        },
      },
      colorPrimary: {
        "&.MuiCheckbox-root": {
          color: palette.primary.main,
          "&$disabled": {
            color: palette.action.disabled,
          },
        },
      },
      colorSecondary: {
        "&.MuiCheckbox-root": {
          color: palette.secondary.main,
          "&$disabled": {
            color: palette.action.disabled,
          },
        },
      },
    },
    MuiRadio: {
      root: {
        "& .MuiSvgIcon-root": {
          width: 18,
          height: 18,
        },
      },
      colorPrimary: {
        "&.MuiRadio-root": {
          color: palette.primary.main,
          "&$disabled": {
            color: palette.action.disabled,
          },
        },
      },
      colorSecondary: {
        "&.MuiRadio-root": {
          color: palette.secondary.main,
          "&$disabled": {
            color: palette.action.disabled,
          },
        },
      },
    },
    MuiTabs: {
      root: {
        minHeight: 40,
      },
      indicator: {
        height: 3,
      },
    },
    MuiTab: {
      root: {
        minHeight: 70,
        marginRight: spacing * 5,
        textTransform: "none",
        fontWeight: 400,
        fontSize: 16,
        letterSpacing: 3,

        "&:last-child": {
          marginRight: 0,
        },

        "&.MuiButtonBase-root": {
          minWidth: 0,
          padding: 0,
        },

        "&.Mui-selected .MuiTab-wrapper": {
          color: palette.primary.contrastText,
          fontWeight: "bold",
        },
      },
      wrapper: {},
      textColorPrimary: {
        color: palette.text.secondary,
      },
      textColorSecondary: {
        color: palette.text.secondary,
      },
    },
    MuiStepper: {
      root: {
        background: "none",
      },
    },
    // Labs components aren't typed properly, so cast overrides
    MuiAutocomplete: {
      endAdornment: {
        "& .MuiSvgIcon-root": {
          fontSize: 20,
        },
      },
      inputRoot: {
        '&[class*="MuiOutlinedInput-root"]': {
          "& $endAdornment": {
            right: 6,
          },
        },
      },
    },
  }
}
