export interface Branding {
	paywall: {
		/** Colors for enterprise are temporary and will be removed when the enterprise license is removed */
		enterprise: {
			background: string;
			border: string;
		};
		premium: {
			background: string;
			divider: string;
			border: string;
		};
	};
	badge: {
		/** Colors for enterprise are temporary and will be removed when the enterprise license is removed */
		enterprise: {
			background: string;
			border: string;
			text: string;
		};
		premium: {
			background: string;
			border: string;
			text: string;
		};
	};
}
