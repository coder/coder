import type { NewTheme } from "../experimental";
import colors from "../tailwindColors";

export default {
  l1: {
    background: colors.gray[950],
    outline: colors.gray[700],
    text: colors.white,
    fill: {
      solid: colors.gray[600],
      outline: colors.gray[600],
      text: colors.white,
    },
  },

  l2: {
    background: colors.gray[900],
    outline: colors.gray[700],
    text: colors.gray[50],
    fill: {
      solid: colors.gray[500],
      outline: colors.gray[500],
      text: colors.white,
    },
    disabled: {
      background: colors.gray[900],
      outline: colors.zinc[700],
      text: colors.gray[200],
      fill: {
        solid: colors.gray[500],
        outline: colors.gray[500],
        text: colors.white,
      },
    },
    hover: {
      background: colors.gray[800],
      outline: colors.gray[600],
      text: colors.white,
      fill: {
        solid: colors.zinc[400],
        outline: colors.zinc[400],
        text: colors.white,
      },
    },
  },
} satisfies NewTheme;
