import type { Meta, StoryObj } from "@storybook/react";
import { FeatureStageBadge } from "./FeatureStageBadge";

const meta: Meta<typeof FeatureStageBadge> = {
	title: "components/FeatureStageBadge",
	component: FeatureStageBadge,
	args: {
		contentType: "beta",
	},
};

export default meta;
type Story = StoryObj<typeof FeatureStageBadge>;

export const MediumBeta: Story = {
	args: {
		size: "md",
	},
};

export const SmallBeta: Story = {
	args: {
		size: "sm",
	},
};

export const LargeBeta: Story = {
	args: {
		size: "lg",
	},
};

export const MediumExperimental: Story = {
	args: {
		size: "md",
		contentType: "experimental",
	},
};
