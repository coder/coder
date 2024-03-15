import type { Roles } from "../roles";
import colors from "../tailwindColors";

export default {
  danger: {
    background: colors.orange[950],
    outline: colors.orange[500],
    text: colors.orange[50],
    fill: {
      solid: colors.orange[500],
      outline: colors.orange[400],
      text: colors.white,
    },
    disabled: {
      background: colors.orange[950],
      outline: colors.orange[800],
      text: colors.orange[200],
      fill: {
        solid: colors.orange[800],
        outline: colors.orange[800],
        text: colors.white,
      },
    },
    hover: {
      background: colors.orange[900],
      outline: colors.orange[500],
      text: colors.white,
      fill: {
        solid: colors.orange[500],
        outline: colors.orange[500],
        text: colors.white,
      },
    },
  },
  error: {
    background: colors.red[950],
    outline: colors.red[600],
    text: colors.red[50],
    fill: {
      solid: colors.red[400],
      outline: colors.red[400],
      text: colors.white,
    },
  },
  warning: {
    background: colors.amber[950],
    outline: colors.amber[300],
    text: colors.amber[50],
    fill: {
      solid: colors.amber[500],
      outline: colors.amber[500],
      text: colors.white,
    },
  },
  notice: {
    background: colors.yellow[950],
    outline: colors.yellow[200],
    text: colors.yellow[50],
    fill: {
      solid: colors.yellow[500],
      outline: colors.yellow[500],
      text: colors.white,
    },
  },
  info: {
    background: colors.blue[950],
    outline: colors.blue[400],
    text: colors.blue[50],
    fill: {
      solid: colors.blue[500],
      outline: colors.blue[600],
      text: colors.white,
    },
  },
  success: {
    background: colors.green[950],
    outline: colors.green[500],
    text: colors.green[50],
    fill: {
      solid: colors.green[600],
      outline: colors.green[600],
      text: colors.white,
    },
    disabled: {
      background: colors.green[950],
      outline: colors.green[800],
      text: colors.green[200],
      fill: {
        solid: colors.green[800],
        outline: colors.green[800],
        text: colors.white,
      },
    },
    hover: {
      background: colors.green[900],
      outline: colors.green[500],
      text: colors.white,
      fill: {
        solid: colors.green[500],
        outline: colors.green[500],
        text: colors.white,
      },
    },
  },
  active: {
    background: colors.sky[950],
    outline: colors.sky[500],
    text: colors.sky[50],
    fill: {
      solid: colors.sky[600],
      outline: colors.sky[400],
      text: colors.white,
    },
    disabled: {
      background: colors.sky[950],
      outline: colors.sky[800],
      text: colors.sky[200],
      fill: {
        solid: colors.sky[800],
        outline: colors.sky[800],
        text: colors.white,
      },
    },
    hover: {
      background: colors.sky[900],
      outline: colors.sky[500],
      text: colors.white,
      fill: {
        solid: colors.sky[500],
        outline: colors.sky[500],
        text: colors.white,
      },
    },
  },
  inactive: {
    background: colors.zinc[950],
    outline: colors.zinc[500],
    text: colors.zinc[50],
    fill: {
      solid: colors.zinc[400],
      outline: colors.zinc[400],
      text: colors.white,
    },
  },
  preview: {
    background: colors.violet[950],
    outline: colors.violet[500],
    text: colors.violet[50],
    fill: {
      solid: colors.violet[400],
      outline: colors.violet[400],
      text: colors.white,
    },
  },
} satisfies Roles;
