import { createMuiTheme } from "@material-ui/core/styles"
import { PaletteOptions } from "@material-ui/core/styles/createPalette"
import { Overrides } from "@material-ui/core/styles/overrides"
import { borderRadius } from "./constants"
import { getOverrides } from "./overrides"
import { darkPalette } from "./palettes"
import { props } from "./props"
import { typography } from "./typography"

const makeTheme = (palette: PaletteOptions) => {
  return createMuiTheme({
    palette,
    typography,
    shape: {
      borderRadius,
    },
    props,
    overrides: getOverrides(palette) as Overrides,
  })
}

export const dark = makeTheme(darkPalette)
