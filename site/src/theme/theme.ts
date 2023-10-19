import colors from "./tailwind";

export interface CoderTheme {
  primary: Role; // page background, things which sit at the "root level"
  secondary: InteractiveRole; // sidebars, table headers, navigation
  tertiary: InteractiveRole; // buttons, inputs
  modal: Role; // modals/popovers/dropdowns

  roles: {
    danger: InteractiveRole; // delete, immutable parameters, stuff that sucks to fix
    error: Role; // something went wrong
    warning: Role; // something is amiss
    notice: Role; // like info, but actionable. "this is fine, but you may want to..."
    info: Role; // just sharing :)
    success: InteractiveRole; // yay!! it's working!!
    active: Role; // selected items, focused inputs,
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

export const darkReal = {
  primary: {
    background: colors.gray[950],
    outline: colors.gray[700],
    fill: "#f00",
    text: colors.white,
  },
  secondary: {
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
  tertiary: {
    background: colors.gray[800],
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
      fill: "#f00",
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
      fill: "#f00",
      text: colors.sky[50],
    },
  },
} satisfies CoderTheme;

export const dark = {
  primary: {
    background: colors.gray[300],
    outline: colors.gray[400],
    fill: "#f00",
    text: "#000",
  },
  secondary: {
    background: colors.gray[200],
    outline: colors.gray[400],
    fill: "#f00",
    text: "#000",
    disabled: {
      background: "#f00",
      outline: "#f00",
      fill: "#f00",
      text: colors.gray[800],
    },
    hover: {
      background: "#f00",
      outline: "#f00",
      fill: "#f00",
      text: "#000",
    },
  },
  tertiary: {
    background: colors.gray[100],
    outline: colors.gray[400],
    fill: "#f00",
    text: "#000",
    disabled: {
      background: "#f00",
      outline: "#f00",
      fill: "#f00",
      text: colors.gray[800],
    },
    hover: {
      background: "#f00",
      outline: "#f00",
      fill: "#f00",
      text: colors.white,
    },
  },
  modal: {
    background: colors.gray[50],
    outline: "#f00",
    fill: "#f00",
    text: colors.white,
  },

  roles: {
    danger: {
      background: colors.orange[50],
      outline: colors.orange[600],
      fill: colors.orange[400],
      text: colors.orange[800],
      disabled: {
        background: colors.orange[50],
        outline: colors.orange[600],
        fill: colors.orange[400],
        text: colors.orange[700],
      },
      hover: {
        background: colors.orange[100],
        outline: colors.orange[500],
        fill: colors.orange[500],
        text: colors.orange[800],
      },
    },
    error: {
      background: colors.red[50],
      outline: colors.red[600],
      fill: colors.red[500],
      text: colors.red[800],
    },
    warning: {
      background: colors.amber[50],
      outline: colors.amber[600],
      fill: "#f00",
      text: colors.amber[800],
    },
    notice: {
      background: colors.yellow[50],
      outline: colors.yellow[700],
      fill: "#f00",
      text: colors.yellow[800],
    },
    info: {
      background: colors.blue[50],
      outline: colors.blue[600],
      fill: colors.blue[500],
      text: colors.blue[800],
    },
    success: {
      background: colors.green[50],
      outline: colors.green[500],
      fill: colors.green[600],
      text: colors.green[800],
      disabled: {
        background: colors.green[50],
        outline: colors.green[600],
        fill: colors.green[800],
        text: colors.green[800],
      },
      hover: {
        background: colors.green[100],
        outline: colors.green[500],
        fill: colors.green[500],
        text: colors.green[800],
      },
    },
    active: {
      background: colors.sky[50],
      outline: colors.sky[400],
      fill: "#f00",
      text: colors.sky[800],
    },
  },
} satisfies CoderTheme;
