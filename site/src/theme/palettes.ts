import { PaletteOptions } from "@material-ui/core/styles/createPalette"
import { colors } from "./colors"

export const darkPalette: PaletteOptions = {
  type: "dark",
  primary: {
    main: colors.blue[7],
    contrastText: colors.gray[3],
    light: colors.blue[6],
    dark: colors.blue[9],
  },
  secondary: {
    main: colors.green[11],
    contrastText: colors.gray[3],
    dark: colors.indigo[7],
  },
  background: {
    default: colors.gray[15],
    paper: colors.gray[14],
  },
  text: {
    primary: colors.gray[3],
    secondary: colors.gray[5],
  },
  divider: colors.gray[13],
  warning: {
    main: colors.orange[11],
    dark: colors.orange[15],
  },
  success: {
    main: colors.green[11],
    dark: colors.green[15],
  },
  info: {
    main: colors.blue[11],
    dark: colors.blue[15],
  },
  error: {
    main: colors.red[11],
    dark: colors.red[15],
  },
}
