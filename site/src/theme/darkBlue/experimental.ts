import { type NewTheme } from "../experimental";
import colors from "../tailwindColors";

export default {
  l1: {
    background: colors.gray[950],
    outline: colors.gray[700],
    fill: colors.gray[600],
    text: colors.white,
  },

  l2: {
    background: colors.gray[900],
    outline: colors.gray[700],
    fill: colors.gray[500],
    text: colors.gray[50],
    disabled: {
      background: colors.gray[900],
      outline: colors.zinc[700],
      fill: colors.gray[500],
      text: colors.gray[200],
    },
    hover: {
      background: colors.gray[800],
      outline: colors.gray[600],
      fill: colors.zinc[400],
      text: colors.white,
    },
  },

  roles: {
    danger: {
      background: colors.orange[950],
      outline: colors.orange[600],
      fill: colors.orange[600],
      text: colors.orange[50],
      disabled: {
        background: colors.orange[950],
        outline: colors.orange[800],
        fill: colors.orange[800],
        text: colors.orange[200],
      },
      hover: {
        background: colors.orange[900],
        outline: colors.orange[500],
        fill: colors.orange[500],
        text: colors.white,
      },
    },
    error: {
      background: colors.red[950],
      outline: colors.red[500],
      fill: colors.red[600],
      text: colors.red[50],
    },
    warning: {
      background: colors.amber[950],
      outline: colors.amber[300],
      fill: colors.amber[500],
      text: colors.amber[50],
    },
    notice: {
      background: colors.yellow[950],
      outline: colors.yellow[200],
      fill: colors.yellow[500],
      text: colors.yellow[50],
    },
    info: {
      background: colors.blue[950],
      outline: colors.blue[400],
      fill: colors.blue[500],
      text: colors.blue[50],
    },
    success: {
      background: colors.green[950],
      outline: colors.green[500],
      fill: colors.green[600],
      text: colors.green[50],
      disabled: {
        background: colors.green[950],
        outline: colors.green[800],
        fill: colors.green[800],
        text: colors.green[200],
      },
      hover: {
        background: colors.green[900],
        outline: colors.green[500],
        fill: colors.green[500],
        text: colors.white,
      },
    },
    active: {
      background: colors.sky[950],
      outline: colors.sky[500],
      fill: colors.sky[600],
      text: colors.sky[50],
      disabled: {
        background: colors.sky[950],
        outline: colors.sky[800],
        fill: colors.sky[800],
        text: colors.sky[200],
      },
      hover: {
        background: colors.sky[900],
        outline: colors.sky[500],
        fill: colors.sky[500],
        text: colors.white,
      },
    },
    preview: {
      background: colors.violet[950],
      outline: colors.violet[500],
      fill: colors.violet[400],
      text: colors.violet[50],
    },
  },
} satisfies NewTheme;
