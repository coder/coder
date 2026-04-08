import type { Branding } from "../branding";
import colors from "../tailwindColors";

const branding: Branding = {
	enterprise: {
		background: colors.purple[950],
		divider: colors.purple[900],
		border: colors.purple[400],
		text: colors.purple[50],
	},
	premium: {
		background: colors.violet[950],
		divider: colors.violet[900],
		border: colors.violet[400],
		text: colors.violet[50],
	},

	featureStage: {
		background: colors.purple[950],
		divider: colors.purple[900],
		border: colors.purple[400],
		text: colors.purple[400],

		hover: {
			background: colors.purple[900],
			divider: colors.purple[800],
			border: colors.purple[300],
			text: colors.purple[300],
		},
	},
};

export default branding;
