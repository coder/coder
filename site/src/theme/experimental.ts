import colors from "./tailwind";

export type ThemeRole = keyof NewTheme["roles"];

export interface NewTheme {
  l1: Role; // page background, things which sit at the "root level"
  l2: InteractiveRole; // sidebars, table headers, navigation
  l3: InteractiveRole; // buttons, inputs
  modal: Role; // modals/popovers/dropdowns

  roles: {
    danger: InteractiveRole; // delete, immutable parameters, stuff that sucks to fix
    error: Role; // something went wrong
    warning: Role; // something is amiss
    notice: Role; // like info, but actionable. "this is fine, but you may want to..."
    info: Role; // just sharing :)
    success: InteractiveRole; // yay!! it's working!!
    active: Role; // selected items, focused inputs, in progress
  };
}

export interface Role {
  background: string;
  outline: string;
  fill: string;
  // contrastOutline?: string;
  text: string;
}

export interface InteractiveRole extends Role {
  disabled: Role;
  hover: Role;
}

export const dark: NewTheme = {
  l1: {
    background: colors.gray[950],
    outline: colors.gray[700],
    fill: colors.gray[600],
    text: colors.white,
  },

  l2: {
    background: colors.gray[900],
    outline: colors.gray[700],
    fill: "#f00",
    text: colors.white,
    disabled: {
      background: "#f00",
      outline: "#f00",
      fill: "#f00",
      text: colors.gray[200],
    },
    hover: {
      background: "#f00",
      outline: "#f00",
      fill: "#f00",
      text: colors.white,
    },
  },

  l3: {
    background: colors.gray[800],
    outline: colors.gray[700],
    fill: colors.gray[600],
    text: colors.white,
    disabled: {
      background: "#f00",
      outline: "#f00",
      fill: "#f00",
      text: colors.gray[200],
    },
    hover: {
      background: "#f00",
      outline: "#f00",
      fill: "#f00",
      text: colors.white,
    },
  },

  modal: {
    background: "#f00",
    outline: "#f00",
    fill: "#f00",
    text: colors.white,
  },

  roles: {
    danger: {
      background: colors.orange[950],
      outline: colors.orange[500],
      fill: colors.orange[600],
      text: colors.orange[50],
      disabled: {
        background: colors.orange[950],
        outline: colors.orange[600],
        fill: colors.orange[800],
        text: colors.orange[200],
      },
      hover: {
        background: colors.orange[900],
        outline: colors.orange[500],
        fill: colors.orange[500],
        text: colors.orange[50],
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
      fill: "#f00",
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
      fill: "#f00",
      text: colors.blue[50],
    },
    success: {
      background: colors.green[950],
      outline: colors.green[500],
      fill: colors.green[600],
      text: colors.green[50],
      disabled: {
        background: colors.green[950],
        outline: colors.green[600],
        fill: colors.green[800],
        text: colors.green[200],
      },
      hover: {
        background: colors.green[900],
        outline: colors.green[500],
        fill: colors.green[500],
        text: colors.green[50],
      },
    },
    active: {
      background: colors.sky[950],
      outline: colors.sky[500],
      fill: colors.sky[600],
      text: colors.sky[50],
    },
  },
};
