import { createMuiTheme } from "@material-ui/core/styles"
import { PaletteOptions } from "@material-ui/core/styles/createPalette"
import { borderRadius } from "./constants"
import { getOverrides } from "./overrides"
import { darkPalette } from "./palettes"
import { props } from "./props"
import { typography } from "./typography"

const makeTheme = (palette: PaletteOptions) => {
  const theme = createMuiTheme({
    palette,
    typography,
    shape: {
      borderRadius,
    },
    props,
  })

  theme.overrides = getOverrides(theme)

  return theme
}

export const dark = makeTheme(darkPalette)
