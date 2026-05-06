import type { NewTheme } from "../experimental";
import colors from "../tailwindColors";

// l1 = page background, l2 = raised surface (cards, dialogs, sidebar).
// We layer on the indigo axis so the gradient feels like a sky rather
// than the flat zinc grey of the default dark theme.
export default {
	l1: {
		background: "#0d0c27",
		outline: "#2d2956",
		text: colors.violet[50],
		fill: {
			solid: colors.indigo[500],
			outline: colors.indigo[400],
			text: colors.white,
		},
	},

	l2: {
		background: "#191935",
		outline: "#2d2956",
		text: colors.violet[50],
		fill: {
			solid: colors.teal[500],
			outline: colors.teal[400],
			text: colors.indigo[950],
		},
		disabled: {
			background: "#191935",
			outline: "#2d2956",
			text: colors.indigo[300],
			fill: {
				solid: colors.indigo[700],
				outline: colors.indigo[700],
				text: colors.white,
			},
		},
		hover: {
			background: "#22214a",
			outline: colors.indigo[400],
			text: colors.white,
			fill: {
				solid: colors.teal[400],
				outline: colors.teal[300],
				text: colors.indigo[950],
			},
		},
	},

	pillDefault: {
		background: "#22214a",
		outline: colors.indigo[600],
		text: colors.violet[100],
	},
} satisfies NewTheme;
