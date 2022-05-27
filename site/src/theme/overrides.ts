import { PaletteOptions, SimplePaletteColorOptions } from "@material-ui/core/styles/createPalette"
import { MONOSPACE_FONT_FAMILY } from "./constants"

// eslint-disable-next-line @typescript-eslint/explicit-module-boundary-types
export const getOverrides = (palette: PaletteOptions) => {
  return {
    MuiAvatar: {
      root: {
        width: 32,
        height: 32,
        fontSize: 24,
        border: `1px solid ${palette.divider}`,
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
        backgroundColor: "#151515",
        "&:hover": {
          boxShadow: "none",
          backgroundColor: "#000000",
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
        border: `1px solid ${palette.divider}`,

        "& td": {
          paddingTop: 16,
          paddingBottom: 16,
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
        "&:first-child": {
          paddingLeft: 32,
        },
        "&:last-child": {
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
        "& input:-webkit-autofill": {
          WebkitBoxShadow: `0 0 0 1000px ${palette.background?.paper} inset`,
        },
        "&:hover .MuiOutlinedInput-notchedOutline": {
          borderColor: (palette.primary as SimplePaletteColorOptions).light,
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
  }
}
