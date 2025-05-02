import type { Branding } from "../branding";
import colors from "../tailwindColors";

const branding: Branding = {
	enterprise: {
		background: colors.blue[100],
		divider: colors.blue[300],
		border: colors.blue[600],
		text: colors.blue[950],
	},
	premium: {
		background: colors.violet[100],
		divider: colors.violet[300],
		border: colors.violet[600],
		text: colors.violet[950],
	},

	featureStage: {
		background: colors.sky[50],
		divider: colors.sky[100],
		border: colors.sky[700],
		text: colors.sky[700],

		hover: {
			background: colors.white,
			divider: colors.zinc[100],
			border: colors.sky[700],
			text: colors.sky[700],
		},
	},
};

export default branding;
