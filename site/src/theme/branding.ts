export type Branding = Readonly<{
	enterprise: Readonly<{
		background: string;
		divider: string;
		border: string;
		text: string;
	}>;

	premium: Readonly<{
		background: string;
		divider: string;
		border: string;
		text: string;
	}>;

	featureStage: Readonly<{
		background: string;
		divider: string;
		border: string;
		text: string;

		hover: Readonly<{
			background: string;
			divider: string;
			border: string;
			text: string;
		}>;
	}>;
}>;
