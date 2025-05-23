import type { NewTheme } from "../experimental";
import colors from "../tailwindColors";

export default {
	l1: {
		background: colors.purple[950],
		outline: colors.purple[700],
		text: colors.white,
		fill: {
			solid: colors.purple[600],
			outline: colors.purple[600],
			text: colors.white,
		},
	},

	l2: {
		background: colors.purple[900],
		outline: colors.purple[700],
		text: colors.zinc[50],
		fill: {
			solid: colors.purple[500],
			outline: colors.purple[500],
			text: colors.white,
		},
		disabled: {
			background: colors.purple[900],
			outline: colors.purple[700],
			text: colors.zinc[200],
			fill: {
				solid: colors.purple[500],
				outline: colors.purple[500],
				text: colors.white,
			},
		},
		hover: {
			background: colors.purple[800],
			outline: colors.purple[600],
			text: colors.white,
			fill: {
				solid: colors.purple[400],
				outline: colors.purple[400],
				text: colors.white,
			},
		},
	},

	pillDefault: {
		background: colors.purple[800],
		outline: colors.purple[700],
		text: colors.zinc[200],
	},
} satisfies NewTheme;
