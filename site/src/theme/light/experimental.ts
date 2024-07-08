import type { NewTheme } from "../experimental";
import colors from "../tailwindColors";

export default {
  l1: {
    background: colors.gray[50],
    outline: colors.gray[300],
    text: colors.black,
    fill: {
      solid: colors.gray[700],
      outline: colors.gray[700],
      text: colors.white,
    },
  },

  l2: {
    background: colors.gray[100],
    outline: colors.gray[500],
    text: colors.gray[950],
    fill: {
      solid: colors.gray[500],
      outline: colors.gray[500],
      text: colors.white,
    },
    disabled: {
      background: colors.gray[100],
      outline: colors.gray[500],
      text: colors.gray[800],
      fill: {
        solid: colors.gray[500],
        outline: colors.gray[500],
        text: colors.white,
      },
    },
    hover: {
      background: colors.gray[200],
      outline: colors.gray[700],
      text: colors.black,
      fill: {
        solid: colors.zinc[600],
        outline: colors.zinc[600],
        text: colors.white,
      },
    },
  },
} satisfies NewTheme;
