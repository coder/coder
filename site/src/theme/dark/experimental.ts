import { type NewTheme } from "../experimental";
import colors from "../tailwindColors";

export default {
  l1: {
    background: colors.zinc[950],
    outline: colors.zinc[700],
    text: colors.white,
    fill: {
      solid: colors.zinc[600],
      outline: colors.zinc[600],
      text: colors.white,
    },
  },

  l2: {
    background: colors.zinc[900],
    outline: colors.zinc[700],
    text: colors.zinc[50],
    fill: {
      solid: colors.zinc[500],
      outline: colors.zinc[500],
      text: colors.white,
    },
    disabled: {
      background: colors.gray[900],
      outline: colors.zinc[700],
      text: colors.zinc[200],
      fill: {
        solid: colors.zinc[500],
        outline: colors.zinc[500],
        text: colors.white,
      },
    },
    hover: {
      background: colors.zinc[800],
      outline: colors.zinc[600],
      text: colors.white,
      fill: {
        solid: colors.zinc[400],
        outline: colors.zinc[400],
        text: colors.white,
      },
    },
  },
} satisfies NewTheme;
