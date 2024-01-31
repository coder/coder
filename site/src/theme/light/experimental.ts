import { type NewTheme } from "../experimental";
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

  roles: {
    danger: {
      background: colors.orange[50],
      outline: colors.orange[400],
      text: colors.orange[950],
      fill: {
        solid: colors.orange[600],
        outline: colors.orange[600],
        text: colors.white,
      },
      disabled: {
        background: colors.orange[50],
        outline: colors.orange[800],
        text: colors.orange[800],
        fill: {
          solid: colors.orange[800],
          outline: colors.orange[800],
          text: colors.white,
        },
      },
      hover: {
        background: colors.orange[100],
        outline: colors.orange[500],
        text: colors.black,
        fill: {
          solid: colors.orange[500],
          outline: colors.orange[500],
          text: colors.white,
        },
      },
    },
    error: {
      background: colors.red[100],
      outline: colors.red[500],
      text: colors.red[950],
      fill: {
        solid: colors.red[600],
        outline: colors.red[600],
        text: colors.white,
      },
    },
    warning: {
      background: colors.amber[50],
      outline: colors.amber[300],
      text: colors.amber[950],
      fill: {
        solid: colors.amber[500],
        outline: colors.amber[500],
        text: colors.white,
      },
    },
    notice: {
      background: colors.yellow[50],
      outline: colors.yellow[600],
      text: colors.yellow[950],
      fill: {
        solid: colors.yellow[500],
        outline: colors.yellow[500],
        text: colors.white,
      },
    },
    info: {
      background: colors.blue[50],
      outline: colors.blue[400],
      text: colors.blue[950],
      fill: {
        solid: colors.blue[600],
        outline: colors.blue[600],
        text: colors.white,
      },
    },
    success: {
      background: colors.green[50],
      outline: colors.green[500],
      text: colors.green[950],
      fill: {
        solid: colors.green[600],
        outline: colors.green[600],
        text: colors.white,
      },
      disabled: {
        background: colors.green[50],
        outline: colors.green[800],
        text: colors.green[800],
        fill: {
          solid: colors.green[800],
          outline: colors.green[800],
          text: colors.white,
        },
      },
      hover: {
        background: colors.green[100],
        outline: colors.green[500],
        text: colors.black,
        fill: {
          solid: colors.green[500],
          outline: colors.green[500],
          text: colors.white,
        },
      },
    },
    active: {
      background: colors.sky[100],
      outline: colors.sky[500],
      text: colors.sky[950],
      fill: {
        solid: colors.sky[600],
        outline: colors.sky[600],
        text: colors.white,
      },
      disabled: {
        background: colors.sky[50],
        outline: colors.sky[800],
        text: colors.sky[200],
        fill: {
          solid: colors.sky[800],
          outline: colors.sky[800],
          text: colors.white,
        },
      },
      hover: {
        background: colors.sky[200],
        outline: colors.sky[400],
        text: colors.black,
        fill: {
          solid: colors.sky[500],
          outline: colors.sky[500],
          text: colors.white,
        },
      },
    },
    inactive: {
      background: colors.gray[100],
      outline: colors.gray[400],
      text: colors.gray[950],
      fill: {
        solid: colors.gray[600],
        outline: colors.gray[600],
        text: colors.white,
      },
    },
    preview: {
      background: colors.violet[50],
      outline: colors.violet[500],
      text: colors.violet[950],
      fill: {
        solid: colors.violet[600],
        outline: colors.violet[600],
        text: colors.white,
      },
    },
  },
} satisfies NewTheme;
