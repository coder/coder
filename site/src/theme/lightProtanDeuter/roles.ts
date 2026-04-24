import type { Roles } from "../roles";
import colors from "../tailwindColors";

// Protanopia and deuteranopia compress the red/green channel, so semantic
// "good/bad" pairs that rely on green vs red need a different axis. We
// base the accents on the Okabe-Ito CVD-safe palette: vermilion
// (orange-600) for destructive states, sky blue for positive/active
// states, and violet for preview. Warning stays amber because it reads as
// clearly different from blue under protan/deuter.
const roles: Roles = {
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
		background: colors.orange[100],
		outline: colors.orange[500],
		text: colors.orange[950],
		fill: {
			solid: colors.orange[600],
			outline: colors.orange[600],
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
		background: colors.blue[50],
		outline: colors.blue[400],
		text: colors.blue[950],
		fill: {
			solid: colors.blue[700],
			outline: colors.blue[600],
			text: colors.white,
		},
	},
	info: {
		background: colors.zinc[50],
		outline: colors.zinc[400],
		text: colors.zinc[950],
		fill: {
			solid: colors.zinc[700],
			outline: colors.zinc[600],
			text: colors.white,
		},
	},
	// Success uses sky blue so it is distinguishable from `error` (orange)
	// under protanopia and deuteranopia.
	success: {
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
			text: colors.sky[800],
			fill: {
				solid: colors.sky[800],
				outline: colors.sky[800],
				text: colors.white,
			},
		},
		hover: {
			background: colors.sky[200],
			outline: colors.sky[500],
			text: colors.black,
			fill: {
				solid: colors.sky[500],
				outline: colors.sky[500],
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
};

export default roles;
