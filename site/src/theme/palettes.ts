import { PaletteOptions } from "@material-ui/core/styles/createPalette"

export const darkPalette: PaletteOptions = {
  type: "dark",
  primary: {
    main: "hsl(215, 81%, 63%)",
    contrastText: "hsl(218, 44%, 92%)",
    light: "hsl(215, 83%, 70%)",
    dark: "hsl(215, 74%, 51%)",
  },
  secondary: {
    main: "hsl(142, 64%, 34%)",
    contrastText: "hsl(218, 44%, 92%)",
    dark: "hsl(233, 73%, 63%)",
  },
  background: {
    default: "hsl(222, 38%, 14%)",
    paper: "hsl(222, 32%, 19%)",
  },
  text: {
    primary: "hsl(218, 44%, 92%)",
    secondary: "hsl(218, 32%, 77%)",
  },
  divider: "hsl(221, 32%, 26%)",
  warning: {
    main: "hsl(20, 79%, 53%)",
  },
  success: {
    main: "hsl(142, 58%, 41%)",
  },
}
