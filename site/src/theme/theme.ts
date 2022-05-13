import { createMuiTheme } from "@material-ui/core/styles"
import { borderRadius } from "./constants"
import { CustomPalette, darkPalette, lightPalette } from "./palettes"
import { props } from "./props"
import { typography } from "./typography"

const makeTheme = (palette: CustomPalette) => {
  return createMuiTheme({
    palette,
    typography,
    shape: {
      borderRadius,
    },
    props,
  })
}

export const light = makeTheme(lightPalette)
export const dark = makeTheme(darkPalette)
