export type ThemeRole = keyof NewTheme["roles"];

export type InteractiveThemeRole = keyof {
  [K in keyof NewTheme["roles"] as NewTheme["roles"][K] extends InteractiveRole
    ? K
    : never]: unknown;
};

export interface NewTheme {
  l1: Role; // page background, things which sit at the "root level"
  l2: InteractiveRole; // sidebars, table headers, navigation
  l3: InteractiveRole; // buttons, inputs

  roles: {
    danger: InteractiveRole; // delete, immutable parameters, stuff that sucks to fix
    error: Role; // something went wrong
    warning: Role; // something is amiss
    notice: Role; // like info, but actionable. "this is fine, but you may want to..."
    info: Role; // just sharing :)
    success: InteractiveRole; // yay!! it's working!!
    active: InteractiveRole; // selected items, focused inputs, in progress
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
