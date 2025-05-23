import type { Branding } from "../branding";
import colors from "../tailwindColors";

const branding: Branding = {
	enterprise: {
		background: colors.blue[950],
		divider: colors.blue[900],
		border: colors.blue[400],
		text: colors.blue[50],
	},
	premium: {
		background: colors.violet[950],
		divider: colors.violet[900],
		border: colors.violet[400],
		text: colors.violet[50],
	},

	featureStage: {
		background: colors.sky[950],
		divider: colors.sky[900],
		border: colors.sky[400],
		text: colors.sky[400],

		hover: {
			background: colors.zinc[950],
			divider: colors.zinc[900],
			border: colors.sky[400],
			text: colors.sky[400],
		},
	},
};

export default branding;
