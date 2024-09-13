import type { Branding } from "../branding";
import colors from "../tailwindColors";

export default {
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
} satisfies Branding;
