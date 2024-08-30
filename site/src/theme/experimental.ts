import type { ColorRole, InteractiveColorRole } from "./colorRoles";

export interface NewTheme {
	l1: ColorRole; // page background, things which sit at the "root level"
	l2: InteractiveColorRole; // sidebars, table headers, navigation
}
