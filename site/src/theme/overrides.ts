import { PaletteOptions, SimplePaletteColorOptions } from "@material-ui/core/styles/createPalette"
import { MONOSPACE_FONT_FAMILY } from "./constants"

// eslint-disable-next-line @typescript-eslint/explicit-module-boundary-types
export const getOverrides = (palette: PaletteOptions) => {
  return {
    MuiCssBaseline: {
      "@global": {
        body: {
          backgroundImage:
            "linear-gradient(to right bottom, hsl(223, 38%, 14%), hsl(221, 53%, 3%))",
          backgroundRepeat: "no-repeat",
          backgroundAttachment: "fixed",
          letterSpacing: "-0.015em",
        },
      },
    },
    MuiAvatar: {
      root: {
        borderColor: palette.divider,
        width: 36,
        height: 36,
        fontSize: 18,
      },
    },
    MuiButton: {
      root: {
        // Prevents a loading button from collapsing!
        minHeight: 42,
        fontWeight: "regular",
        fontFamily: MONOSPACE_FONT_FAMILY,
        fontSize: 16,
        textTransform: "none",
        letterSpacing: "none",
        border: `1px solid ${palette.divider}`,
      },
      contained: {
        boxShadow: "none",
        color: palette.text?.primary,
        backgroundColor: "hsl(223, 27%, 3%)",
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
        fontFamily: MONOSPACE_FONT_FAMILY,
        textTransform: "uppercase",
      },
    },
    MuiTable: {
      root: {
        // Gives the appearance of a border!
        borderRadius: 2,
        background: "hsla(222, 31%, 19%, .5)",

        "& td": {
          paddingTop: 16,
          paddingBottom: 16,
          background: "transparent",
        },
      },
    },
    MuiTableCell: {
      head: {
        color: palette.text?.secondary,
      },
      root: {
        fontFamily: MONOSPACE_FONT_FAMILY,
        fontSize: 16,
        background: palette.background?.paper,
        borderBottom: `1px solid ${palette.divider}`,
        padding: 8,
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
        borderRadius: 2,
      },
    },
    MuiOutlinedInput: {
      root: {
        "& .MuiOutlinedInput-notchedOutline": {
          borderColor: palette.divider,
        },

        "& input:-webkit-autofill": {
          WebkitBoxShadow: `0 0 0 1000px ${palette.background?.paper} inset`,
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
        borderRadius: 2,
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
