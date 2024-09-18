import type { Meta, StoryObj } from "@storybook/react";
import { FeatureStageBadge } from "./FeatureStageBadge";

const meta: Meta<typeof FeatureStageBadge> = {
	title: "components/FeatureStageBadge",
	component: FeatureStageBadge,
	args: {
		type: "beta",
	},
};

export default meta;
type Story = StoryObj<typeof FeatureStageBadge>;

export const SmallInteractiveBeta: Story = {
	args: {
		type: "beta",
		size: "sm",
		variant: "interactive",
	},
};

export const SmallInteractiveExperimental: Story = {
	args: {
		type: "experimental",
		size: "sm",
		variant: "interactive",
	},
};

export const LargeInteractiveBeta: Story = {
	args: {
		type: "beta",
		size: "lg",
		variant: "interactive",
	},
};

export const LargeStaticBeta: Story = {
	args: {
		type: "beta",
		size: "lg",
		variant: "static",
	},
};

// While it's possible, we shouldn't use the xs size with the interactive
// variant, because then the badge might be too small for users with motor
// control issues to get their hand to hover over the target reliably
export const ExtraSmallStaticBeta: Story = {
	args: {
		type: "beta",
		size: "xs",
		variant: "static",
	},
};
