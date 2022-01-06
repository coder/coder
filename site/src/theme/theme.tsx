/**
 * @fileoverview Coder design system
 * @link <https://www.figma.com/file/RPFmHcWDZPRrN7c1vGZdjE/Coder-Design-1.0?node-id=8%3A18558>
 */
import { createMuiTheme } from "@material-ui/core/styles"
import { borderRadius } from "./constants"
import { CustomPalette, darkPalette, lightPalette } from "./palettes"
import { typography } from "./typography"

/**
 * Shared theme configuration
 */
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
