import { type NewTheme } from "../experimental";
import colors from "../tailwind";

export default {
  l1: {
    background: colors.gray[50],
    outline: colors.gray[300],
    fill: colors.gray[700],
    text: colors.black,
  },

  l2: {
    background: colors.gray[100],
    outline: colors.gray[500],
    fill: colors.gray[500],
    text: colors.gray[950],
    disabled: {
      background: colors.gray[100],
      outline: colors.gray[500],
      fill: colors.gray[500],
      text: colors.gray[800],
    },
    hover: {
      background: colors.gray[200],
      outline: colors.gray[700],
      fill: colors.zinc[600],
      text: colors.black,
    },
  },

  roles: {
    danger: {
      background: colors.orange[50],
      outline: colors.orange[400],
      fill: colors.orange[600],
      text: colors.orange[950],
      disabled: {
        background: colors.orange[50],
        outline: colors.orange[800],
        fill: colors.orange[800],
        text: colors.orange[800],
      },
      hover: {
        background: colors.orange[100],
        outline: colors.orange[500],
        fill: colors.orange[500],
        text: colors.black,
      },
    },
    error: {
      background: colors.red[100],
      outline: colors.red[500],
      fill: colors.red[600],
      text: colors.red[950],
    },
    warning: {
      background: colors.amber[50],
      outline: colors.amber[300],
      fill: colors.amber[500],
      text: colors.amber[950],
    },
    notice: {
      background: colors.yellow[50],
      outline: colors.yellow[600],
      fill: colors.yellow[500],
      text: colors.yellow[950],
    },
    info: {
      background: colors.blue[50],
      outline: colors.blue[400],
      fill: colors.blue[600],
      text: colors.blue[950],
    },
    success: {
      background: colors.green[50],
      outline: colors.green[500],
      fill: colors.green[600],
      text: colors.green[950],
      disabled: {
        background: colors.green[50],
        outline: colors.green[800],
        fill: colors.green[800],
        text: colors.green[800],
      },
      hover: {
        background: colors.green[100],
        outline: colors.green[500],
        fill: colors.green[500],
        text: colors.black,
      },
    },
    active: {
      background: colors.sky[100],
      outline: colors.sky[500],
      fill: colors.sky[600],
      text: colors.sky[950],
      disabled: {
        background: colors.sky[50],
        outline: colors.sky[800],
        fill: colors.sky[800],
        text: colors.sky[200],
      },
      hover: {
        background: colors.sky[200],
        outline: colors.sky[400],
        fill: colors.sky[500],
        text: colors.black,
      },
    },
    preview: {
      background: colors.violet[50],
      outline: colors.violet[500],
      fill: colors.violet[600],
      text: colors.violet[950],
    },
  },
} satisfies NewTheme;
