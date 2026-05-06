import type { Roles } from "../roles";
import colors from "../tailwindColors";

// Aurora role palette.
//
// The base "midnight" surfaces are indigo, so role surfaces lean on
// adjacent hues that stay readable against deep indigo:
//   success / active   -> teal (the aurora itself)
//   warning / preview  -> rose / fuchsia (the dawn)
//   error              -> rose (saturated; alarms reliably)
//   info / inactive    -> indigo (sits on the base axis and recedes)
//
// `active` and `success` both live on the teal axis but at different
// shades so they stay distinguishable: `active` uses the brightest mint
// (teal 300) for the running-workspace pulse, while `success` sits on
// the cooler teal 500/600 so a green checkmark still reads as a calm
// "go" rather than a glowing "now".
const roles: Roles = {
	danger: {
		background: colors.rose[950],
		outline: colors.rose[500],
		text: colors.rose[50],
		fill: {
			solid: colors.rose[500],
			outline: colors.rose[400],
			text: colors.white,
		},
		disabled: {
			background: colors.rose[950],
			outline: colors.rose[800],
			text: colors.rose[200],
			fill: {
				solid: colors.rose[800],
				outline: colors.rose[800],
				text: colors.white,
			},
		},
		hover: {
			background: colors.rose[900],
			outline: colors.rose[400],
			text: colors.white,
			fill: {
				solid: colors.rose[400],
				outline: colors.rose[400],
				text: colors.white,
			},
		},
	},
	error: {
		background: colors.rose[950],
		outline: colors.rose[400],
		text: colors.rose[50],
		fill: {
			solid: colors.rose[400],
			outline: colors.rose[400],
			text: colors.white,
		},
	},
	warning: {
		background: colors.amber[950],
		outline: colors.amber[300],
		text: colors.amber[50],
		fill: {
			solid: colors.amber[400],
			outline: colors.amber[400],
			text: colors.indigo[950],
		},
	},
	notice: {
		background: colors.cyan[950],
		outline: colors.cyan[400],
		text: colors.cyan[50],
		fill: {
			solid: colors.cyan[500],
			outline: colors.cyan[600],
			text: colors.white,
		},
	},
	info: {
		background: colors.indigo[950],
		outline: colors.indigo[400],
		text: colors.violet[50],
		fill: {
			solid: colors.indigo[500],
			outline: colors.indigo[600],
			text: colors.white,
		},
	},
	success: {
		background: colors.teal[950],
		outline: colors.teal[500],
		text: colors.teal[50],
		fill: {
			solid: colors.teal[500],
			outline: colors.teal[500],
			text: colors.indigo[950],
		},
		disabled: {
			background: colors.teal[950],
			outline: colors.teal[800],
			text: colors.teal[200],
			fill: {
				solid: colors.teal[800],
				outline: colors.teal[800],
				text: colors.white,
			},
		},
		hover: {
			background: colors.teal[900],
			outline: colors.teal[400],
			text: colors.white,
			fill: {
				solid: colors.teal[400],
				outline: colors.teal[400],
				text: colors.indigo[950],
			},
		},
	},
	active: {
		// "Active" is the running-workspace, currently-selected,
		// primary-call-to-action role. The aurora teal lives here so
		// running workspaces glow against the indigo background.
		background: colors.teal[950],
		outline: colors.teal[400],
		text: colors.teal[50],
		fill: {
			solid: colors.teal[400],
			outline: colors.teal[300],
			text: colors.indigo[950],
		},
		disabled: {
			background: colors.teal[950],
			outline: colors.teal[800],
			text: colors.teal[200],
			fill: {
				solid: colors.teal[800],
				outline: colors.teal[800],
				text: colors.white,
			},
		},
		hover: {
			background: colors.teal[900],
			outline: colors.teal[300],
			text: colors.white,
			fill: {
				solid: colors.teal[300],
				outline: colors.teal[300],
				text: colors.indigo[950],
			},
		},
	},
	inactive: {
		background: colors.indigo[950],
		outline: colors.indigo[500],
		text: colors.violet[50],
		fill: {
			solid: colors.indigo[400],
			outline: colors.indigo[400],
			text: colors.white,
		},
	},
	preview: {
		// Preview/experimental features get the dawn-pink accent so
		// they read as fresh and distinct from the aurora-teal active
		// state.
		background: colors.fuchsia[950],
		outline: colors.fuchsia[400],
		text: colors.fuchsia[50],
		fill: {
			solid: colors.fuchsia[400],
			outline: colors.fuchsia[400],
			text: colors.white,
		},
	},
};

export default roles;
