import type { Meta, StoryObj } from "@storybook/react-vite";
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

export const SmallBeta: Story = {
	args: {
		size: "sm",
		contentType: "beta",
	},
};

export const MediumBeta: Story = {
	args: {
		size: "md",
		contentType: "beta",
	},
};

export const SmallEarlyAccess: Story = {
	args: {
		size: "sm",
		contentType: "early_access",
	},
};

export const MediumEarlyAccess: Story = {
	args: {
		size: "md",
		contentType: "early_access",
	},
};
