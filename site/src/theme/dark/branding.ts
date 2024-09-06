import type { Branding } from "../branding";
import colors from "../tailwindColors";

export default {
	paywall: {
		enterprise: {
			background: colors.sky[950],
			border: colors.sky[400],
		},
		premium: {
			background: colors.violet[950],
			border: colors.violet[400],
		},
	},
	badge: {
		enterprise: {
			background: colors.blue[950],
			border: colors.blue[400],
			text: colors.blue[50],
		},
		premium: {
			background: colors.violet[950],
			border: colors.violet[400],
			text: colors.violet[50],
		},
	},
} satisfies Branding;
