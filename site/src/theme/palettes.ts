import { PaletteOptions } from "@material-ui/core/styles/createPalette"
import { colors } from "./colors"

// Couldn't find a type for this so I made one. We can extend the palette if needed with module augmentation.
export type PaletteIndex = "primary" | "secondary" | "info" | "success" | "error" | "warning"

export const darkPalette: PaletteOptions = {
  type: "dark",
  primary: {
    main: colors.blue[7],
    contrastText: colors.gray[4],
    light: colors.blue[6],
    dark: colors.blue[9],
  },
  secondary: {
    main: colors.gray[11],
    contrastText: colors.gray[4],
    dark: colors.indigo[7],
  },
  background: {
    default: colors.gray[17],
    paper: colors.gray[16],
  },
  text: {
    primary: colors.gray[4],
    secondary: colors.gray[5],
  },
  divider: colors.gray[13],
  warning: {
    light: colors.orange[7],
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
    contrastText: colors.gray[4],
  },
  error: {
    main: colors.red[5],
    dark: colors.red[15],
    contrastText: colors.gray[4],
  },
  action: {
    hover: colors.gray[13],
  },
}
