import { PaletteOptions } from "@material-ui/core/styles/createPalette"

/**
 * Augment MUI Palette with Coder-specific design system
 */

/**
 * Dark theme color palette
 *
 * This maps to our design system at:
 * https://www.figma.com/file/VkXU4873QOsSprMQV02GgR/Design-System?node-id=219%3A40
 */
export const darkPalette: PaletteOptions = {
  type: "dark",
  primary: {
    main: "#409BF4",
    contrastText: "#f8f8f8",
    light: "#79B8FF",
    dark: "#1976D2",
  },
  secondary: {
    main: "#008510",
    contrastText: "#f8f8f8",
  },
  background: {
    default: "#1F1F1F",
    paper: "#292929",
  },
  text: {
    primary: "#F8F8F8",
    secondary: "#C1C1C1",
  },
  divider: "#383838",
  warning: {
    main: "#C16800",
  },
  success: {
    main: "#6BBE00",
  },
}
