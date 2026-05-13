import type { Roles } from "../roles";
import colors from "../tailwindColors";

// Protanopia and deuteranopia compress the red/green channel, so semantic
// "good/bad" pairs that rely on green vs red need a different axis. We
// shift destructive states onto a vermilion/orange hue (Tailwind orange
// scale, inspired by the Okabe-Ito CVD-safe scheme), positive/active
// states onto sky-blue, and warning onto fuchsia so it does not collide
// with destructive states on the orange axis. Preview stays on violet.
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
		background: colors.red[100],
		outline: colors.red[500],
		text: colors.red[950],
		fill: {
			solid: colors.red[600],
			outline: colors.red[600],
			text: colors.white,
		},
	},
	warning: {
		background: colors.fuchsia[50],
		outline: colors.fuchsia[300],
		text: colors.fuchsia[950],
		fill: {
			solid: colors.fuchsia[500],
			outline: colors.fuchsia[500],
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
	// Success uses sky blue so it is distinguishable from `error` (red)
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
