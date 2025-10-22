import type { InteractiveRole, Role } from "./roles";

export interface NewTheme {
	l1: Role; // page background, things which sit at the "root level"
	l2: InteractiveRole; // sidebars, table headers, navigation
	pillDefault: {
		background: string;
		outline: string;
		text: string;
	};
}
