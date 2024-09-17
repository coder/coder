import type { Branding } from "../branding";
import colors from "../tailwindColors";

export default {
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
} satisfies Branding;
