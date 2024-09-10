import type { Branding } from "../branding";
import colors from "../tailwindColors";

export default {
	paywall: {
		premium: {
			background: colors.violet[100],
			divider: colors.violet[300],
			border: colors.violet[600],
		},
	},
	badge: {
		premium: {
			background: colors.violet[50],
			border: colors.violet[400],
			text: colors.violet[950],
		},
	},
} satisfies Branding;
