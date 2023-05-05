import { createTheme, adaptV4Theme } from "@mui/material/styles";
import { PaletteOptions } from "@mui/material/styles/createPalette"
import { borderRadius } from "./constants"
import { getOverrides } from "./overrides"
import { darkPalette } from "./palettes"
import { props } from "./props"
import { typography } from "./typography"
import isChromatic from "chromatic/isChromatic"

const makeTheme = (palette: PaletteOptions) => {
  const theme = createTheme(adaptV4Theme({
    palette,
    typography,
    shape: {
      borderRadius,
    },
    props,
  }))

  // We want to disable transitions during chromatic snapshots
  // https://www.chromatic.com/docs/animations#javascript-animations
  // https://github.com/mui/material-ui/issues/10560#issuecomment-439147374
  if (isChromatic()) {
    theme.transitions.create = () => "none"
  }

  theme.overrides = getOverrides(theme)

  return theme
}

export const dark = makeTheme(darkPalette)
