import { PaletteOptions } from "@material-ui/core/styles/createPalette"
import { MONOSPACE_FONT_FAMILY } from "./constants"

// eslint-disable-next-line @typescript-eslint/explicit-module-boundary-types
export const getOverrides = (palette: PaletteOptions) => {
  return {
    MuiAvatar: {
      root: {
        width: 32,
        height: 32,
        fontSize: 24,
      },
    },
    MuiButton: {
      root: {
        fontWeight: "regular",
        fontFamily: MONOSPACE_FONT_FAMILY,
        fontSize: 16,
        textTransform: "none",
        letterSpacing: "none",
        border: `1px solid ${palette.divider}`,
      },
      contained: {
        boxShadow: "none",
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
        background: palette.background?.paper,
        borderRadius: 2,
      },
    },
    MuiOutlinedInput: {
      root: {
        borderColor: palette.divider,
        "&:hover > .MuiOutlinedInput-notchedOutline": {
          borderColor: palette.divider,
        },
      },
    },
  }
}
