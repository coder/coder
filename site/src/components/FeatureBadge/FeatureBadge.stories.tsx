import type { Meta, StoryObj } from "@storybook/react";
import { FeatureBadge } from "./FeatureBadge";

const meta: Meta<typeof FeatureBadge> = {
	title: "components/FeatureBadge",
	component: FeatureBadge,
};

export default meta;
type Story = StoryObj<typeof FeatureBadge>;

export const Small: Story = {
	args: {
		size: "sm",
	},
};

export const Medium: Story = {
	args: {
		size: "md",
	},
};

export const HighlightedSmall: Story = {
	args: {
		size: "sm",
		highlighted: true,
	},
};

export const HighlightedMedium: Story = {
	args: {
		size: "md",
		highlighted: true,
	},
};
