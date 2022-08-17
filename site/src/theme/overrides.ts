import { Theme } from "@material-ui/core/styles"
import { SimplePaletteColorOptions } from "@material-ui/core/styles/createPalette"
import { Overrides } from "@material-ui/core/styles/overrides"
import { colors } from "./colors"
import { borderRadius, MONOSPACE_FONT_FAMILY } from "./constants"

export const getOverrides = ({ palette, breakpoints }: Theme): Overrides => {
  return {
    MuiCssBaseline: {
      "@global": {
        body: {
          backgroundImage: `linear-gradient(to right bottom, ${colors.gray[15]}, ${colors.gray[17]})`,
          backgroundRepeat: "no-repeat",
          backgroundAttachment: "fixed",
          letterSpacing: "-0.015em",
        },
      },
    },
    MuiAvatar: {
      root: {
        width: 36,
        height: 36,
        fontSize: 18,
      },
      colorDefault: {
        backgroundColor: "#a1adc9",
      },
    },
    MuiButton: {
      root: {
        // Prevents a loading button from collapsing!
        minHeight: 42,
        fontWeight: "normal",
        fontFamily: MONOSPACE_FONT_FAMILY,
        fontSize: 16,
        textTransform: "none",
        letterSpacing: "none",
        border: `1px solid ${palette.divider}`,
      },
      contained: {
        boxShadow: "none",
        color: palette.text.primary,
        backgroundColor: colors.gray[17],
        "&:hover": {
          boxShadow: "none",
          backgroundColor: "#000000",
        },
      },
      sizeSmall: {
        padding: `0 12px`,
        fontSize: 14,
        minHeight: 36,
      },
      iconSizeSmall: {
        width: 16,
        height: 16,
        marginLeft: "0 !important",
        marginRight: 12,
      },
    },
    MuiIconButton: {
      sizeSmall: {
        "& .MuiSvgIcon-root": {
          width: 20,
          height: 20,
        },
      },
    },
    MuiTableHead: {
      root: {
        display: "table-header-group",
        fontFamily: MONOSPACE_FONT_FAMILY,
      },
    },
    MuiTableContainer: {
      root: {
        borderRadius,
        border: `1px solid ${palette.divider}`,
      },
    },
    MuiTable: {
      root: {
        borderCollapse: "collapse",
        border: "none",
        background: colors.gray[15],
        boxShadow: `0 0 0 1px ${colors.gray[15]} inset`,
        overflow: "hidden",

        "& td": {
          paddingTop: 16,
          paddingBottom: 16,
          background: "transparent",
        },

        [breakpoints.down("sm")]: {
          // Random value based on visual adjustments.
          // This is used to avoid line breaking on columns
          minWidth: 1000,
        },
      },
    },

    MuiTableCell: {
      head: {
        fontSize: 14,
        color: colors.gray[5],
        fontWeight: 600,
      },
      root: {
        fontFamily: MONOSPACE_FONT_FAMILY,
        fontSize: 16,
        background: palette.background.paper,
        borderBottom: `1px solid ${palette.divider}`,
        padding: "12px 8px",
        // This targets the first+last td elements, and also the first+last elements
        // of a TableCellLink.
        "&:not(:only-child):first-child, &:not(:only-child):first-child > a": {
          paddingLeft: 32,
        },
        "&:not(:only-child):last-child, &:not(:only-child):last-child > a": {
          paddingRight: 32,
        },
      },
    },
    MuiInputBase: {
      root: {
        borderRadius,
      },
    },
    MuiOutlinedInput: {
      root: {
        "& .MuiOutlinedInput-notchedOutline": {
          borderColor: palette.divider,
        },

        "& input:-webkit-autofill": {
          WebkitBoxShadow: `0 0 0 1000px ${palette.background.paper} inset`,
        },
        "&:hover .MuiOutlinedInput-notchedOutline": {
          borderColor: palette.divider,
        },
      },
    },
    MuiLink: {
      root: {
        color: (palette.primary as SimplePaletteColorOptions).light,
      },
    },
    MuiPaper: {
      root: {
        borderRadius,
        border: `1px solid ${palette.divider}`,
      },
    },
    MuiFormHelperText: {
      contained: {
        marginLeft: 0,
        marginRight: 0,
      },

      marginDense: {
        marginTop: 8,
      },
    },
  }
}
