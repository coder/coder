import type { Branding } from "../branding";
import colors from "../tailwindColors";

// Branding pills. The aurora theme uses fuchsia for premium and teal
// for enterprise so the two licenses stay clearly distinguishable on a
// cool indigo backdrop.
const branding: Branding = {
	enterprise: {
		background: colors.teal[950],
		divider: colors.teal[900],
		border: colors.teal[400],
		text: colors.teal[50],
	},
	premium: {
		background: colors.fuchsia[950],
		divider: colors.fuchsia[900],
		border: colors.fuchsia[400],
		text: colors.fuchsia[50],
	},

	featureStage: {
		background: colors.indigo[950],
		divider: colors.indigo[900],
		border: colors.cyan[300],
		text: colors.cyan[300],

		hover: {
			background: "#0d0c27",
			divider: colors.indigo[900],
			border: colors.cyan[300],
			text: colors.cyan[200],
		},
	},
};

export default branding;
