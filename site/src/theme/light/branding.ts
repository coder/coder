import type { Branding } from "../branding";
import colors from "../tailwindColors";

export default {
	paywall: {
		enterprise: {
			background: colors.sky[100],
			border: colors.sky[600],
		},
		premium: {
			background: colors.violet[100],
			divider: colors.violet[300],
			border: colors.violet[600],
		},
	},
	badge: {
		enterprise: {
			background: colors.blue[50],
			border: colors.blue[400],
			text: colors.blue[950],
		},
		premium: {
			background: colors.violet[50],
			border: colors.violet[400],
			text: colors.violet[950],
		},
	},
} satisfies Branding;
