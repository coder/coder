import type { Role, InteractiveRole } from "./roles";

export interface NewTheme {
  l1: Role; // page background, things which sit at the "root level"
  l2: InteractiveRole; // sidebars, table headers, navigation
}
