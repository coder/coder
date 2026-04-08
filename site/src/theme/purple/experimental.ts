import type { NewTheme } from "../experimental";
import colors from "../tailwindColors";

export default {
	l1: {
		background: "#0d0816",
		outline: "#3d3458",
		text: colors.white,
		fill: {
			solid: "#4a3d6b",
			outline: "#4a3d6b",
			text: colors.white,
		},
	},

	l2: {
		background: "#150e21",
		outline: "#3d3458",
		text: colors.zinc[50],
		fill: {
			solid: "#5a4d7a",
			outline: "#5a4d7a",
			text: colors.white,
		},
		disabled: {
			background: "#120b1e",
			outline: "#3d3458",
			text: colors.zinc[200],
			fill: {
				solid: "#4a3d6b",
				outline: "#4a3d6b",
				text: colors.white,
			},
		},
		hover: {
			background: "#1e1632",
			outline: "#4a3d6b",
			text: colors.white,
			fill: {
				solid: "#6a5d8a",
				outline: "#6a5d8a",
				text: colors.white,
			},
		},
	},

	pillDefault: {
		background: "#1e1632",
		outline: "#3d3458",
		text: colors.zinc[200],
	},
} satisfies NewTheme;
