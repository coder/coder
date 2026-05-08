import type { Roles } from "../roles";
import colors from "../tailwindColors";

// Tritanopia reduces blue/yellow discrimination, so the standard amber
// warning can blur into the sky-blue active/notice accents. Red vs
// green remains intact, so `error` and `danger` keep their red/orange
// axis. We follow GitHub Primer's tritanopia palette and shift
// `success` from green to sky-blue so the success+destructive pair
// stays consistent with the protan-deuter palette and matches the
// diff-addition convention users expect from other tritan-aware tools
// (see primer/primitives' diffBlob.json5: `'dark-tritanopia':
// '{base.color.blue.4}'`). `warning` shifts to fuchsia because amber
// and sky-blue blur together under tritanopia.
const roles: Roles = {
	danger: {
		background: colors.orange[950],
		outline: colors.orange[500],
		text: colors.orange[50],
		fill: {
			solid: colors.orange[500],
			outline: colors.orange[400],
			text: colors.white,
		},
		disabled: {
			background: colors.orange[950],
			outline: colors.orange[800],
			text: colors.orange[200],
			fill: {
				solid: colors.orange[800],
				outline: colors.orange[800],
				text: colors.white,
			},
		},
		hover: {
			background: colors.orange[900],
			outline: colors.orange[500],
			text: colors.white,
			fill: {
				solid: colors.orange[500],
				outline: colors.orange[500],
				text: colors.white,
			},
		},
	},
	error: {
		background: colors.red[950],
		outline: colors.red[600],
		text: colors.red[50],
		fill: {
			solid: colors.red[400],
			outline: colors.red[400],
			text: colors.white,
		},
	},
	// Warning shifts from amber to fuchsia because amber and sky blue blur
	// together under tritanopia.
	warning: {
		background: colors.fuchsia[950],
		outline: colors.fuchsia[300],
		text: colors.fuchsia[50],
		fill: {
			solid: colors.fuchsia[500],
			outline: colors.fuchsia[500],
			text: colors.white,
		},
	},
	notice: {
		background: colors.blue[950],
		outline: colors.blue[400],
		text: colors.blue[50],
		fill: {
			solid: colors.blue[500],
			outline: colors.blue[600],
			text: colors.white,
		},
	},
	info: {
		background: colors.zinc[950],
		outline: colors.zinc[400],
		text: colors.zinc[50],
		fill: {
			solid: colors.zinc[500],
			outline: colors.zinc[600],
			text: colors.white,
		},
	},
	success: {
		background: colors.sky[950],
		outline: colors.sky[500],
		text: colors.sky[50],
		fill: {
			solid: colors.sky[600],
			outline: colors.sky[600],
			text: colors.white,
		},
		disabled: {
			background: colors.sky[950],
			outline: colors.sky[800],
			text: colors.sky[200],
			fill: {
				solid: colors.sky[800],
				outline: colors.sky[800],
				text: colors.white,
			},
		},
		hover: {
			background: colors.sky[900],
			outline: colors.sky[500],
			text: colors.white,
			fill: {
				solid: colors.sky[500],
				outline: colors.sky[500],
				text: colors.white,
			},
		},
	},
	active: {
		background: colors.sky[950],
		outline: colors.sky[500],
		text: colors.sky[50],
		fill: {
			solid: colors.sky[600],
			outline: colors.sky[400],
			text: colors.white,
		},
		disabled: {
			background: colors.sky[950],
			outline: colors.sky[800],
			text: colors.sky[200],
			fill: {
				solid: colors.sky[800],
				outline: colors.sky[800],
				text: colors.white,
			},
		},
		hover: {
			background: colors.sky[900],
			outline: colors.sky[500],
			text: colors.white,
			fill: {
				solid: colors.sky[500],
				outline: colors.sky[500],
				text: colors.white,
			},
		},
	},
	inactive: {
		background: colors.zinc[950],
		outline: colors.zinc[500],
		text: colors.zinc[50],
		fill: {
			solid: colors.zinc[400],
			outline: colors.zinc[400],
			text: colors.white,
		},
	},
	preview: {
		background: colors.violet[950],
		outline: colors.violet[500],
		text: colors.violet[50],
		fill: {
			solid: colors.violet[400],
			outline: colors.violet[400],
			text: colors.white,
		},
	},
};

export default roles;
