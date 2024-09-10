import type { Branding } from "../branding";
import colors from "../tailwindColors";

export default {
	paywall: {
		premium: {
			background: colors.violet[950],
			divider: colors.violet[900],
			border: colors.violet[400],
		},
	},
	badge: {
		premium: {
			background: colors.violet[950],
			border: colors.violet[400],
			text: colors.violet[50],
		},
	},
} satisfies Branding;
