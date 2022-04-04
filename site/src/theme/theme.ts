import { createMuiTheme } from "@material-ui/core/styles"
import { Overrides } from "@material-ui/core/styles/overrides"
import { borderRadius } from "./constants"
import { getOverrides } from "./overrides"
import { CustomPalette, darkPalette, lightPalette } from "./palettes"
import { props } from "./props"
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
    props,
    overrides: getOverrides(palette, breakpoints) as Overrides,
  })
}

export const light = makeTheme(lightPalette)
export const dark = makeTheme(darkPalette)
