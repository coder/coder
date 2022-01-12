import { createMuiTheme } from "@material-ui/core/styles"
import { borderRadius } from "./constants"
import { CustomPalette, darkPalette, lightPalette } from "./palettes"
import { typography } from "./typography"

const makeTheme = (palette: CustomPalette) => {
  // Grab defaults to re-use in overrides
  const { breakpoints } = createMuiTheme()

  return createMuiTheme({
    palette,
    typography,
    shape: {
      borderRadius,
    },
  })
}

export const light = makeTheme(lightPalette)
export const dark = makeTheme(darkPalette)
