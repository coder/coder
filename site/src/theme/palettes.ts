import { Palette } from "@material-ui/core/styles/createPalette"
/**
 * Augment MUI Palette with Coder-specific design system
 */
declare module "@material-ui/core/styles/createPalette" {
  interface Palette {
    navbar: {
      main: string
    }
  }

  interface PaletteOptions {
    navbar: {
      main: string
    }
  }
}
/**
 * CustomPalette implements a minimal subset of MUI Palette interface for our
 * light and dark themes.
 */
export type CustomPalette = Pick<
  Palette,
  "action" | "background" | "divider" | "error" | "info" | "navbar" | "primary" | "secondary" | "text" | "type"
>

/**
 * Light theme color palette, the default
 *
 * This maps to our design system at:
 * https://www.figma.com/file/VkXU4873QOsSprMQV02GgR/Design-System?node-id=3%3A2
 */
export const lightPalette: CustomPalette = {
  type: "light",
  background: {
    default: "#F3F3F3",
    paper: "#FFF",
  },

  primary: {
    main: "#519A54",
    light: "#A2E0A5",
    dark: "#3A783D",
    contrastText: "#FFF",
  },
  info: {
    main: "#000",
    light: "#000",
    dark: "#000",
    contrastText: "#FFF",
  },
  navbar: {
    main: "#242424",
  },
  secondary: {
    main: "#F7CD6F",
    light: "#FFE7A0",
    dark: "#BF9331",
    contrastText: "#FFF",
  },
  error: {
    main: "#DD4764",
    light: "#A14E5E",
    dark: "#912F42",
    contrastText: "#FFF",
  },
  text: {
    primary: "#000",
    secondary: "#747474",
    disabled: "#749367",
    hint: "#749367",
  },
  action: {
    active: "#242424",
    hover: "rgba(0, 0, 0, 0.1)",
    hoverOpacity: 0.08,
    selected: "#D0EFD2",
    disabled: "#DDE2EC",
    disabledBackground: "#F3F3F3",
  },
  divider: "#DDE2EC",
}

/**
 * Dark theme color palette
 *
 * This maps to our design system at:
 * https://www.figma.com/file/VkXU4873QOsSprMQV02GgR/Design-System?node-id=219%3A40
 */
export const darkPalette: CustomPalette = {
  type: "dark",
  primary: lightPalette.primary,
  secondary: lightPalette.secondary,
  info: lightPalette.info,
  error: lightPalette.error,
  navbar: {
    main: "rgb(8, 9, 10)",
  },
  background: {
    default: "rgb(24, 26, 27)",
    paper: "rgb(31, 33, 35)",
  },
  text: {
    primary: "rgba(255, 255, 255, 0.95)",
    secondary: "#BDBDBD",
    disabled: "#BDBDBD",
    hint: "#BDBDBD",
  },
  action: {
    active: "#FFF",
    hover: "rgba(255, 255, 255, 0.1)",
    hoverOpacity: 0.1,
    selected: "rgba(255, 255, 255, 0.2)",
    disabled: "rgba(255, 255, 255, 0.1)",
    disabledBackground: "rgba(255, 255, 255, 0.12)",
  },
  divider: "rgba(255, 255, 255, 0.12)",
}
