import type { Meta, StoryObj } from "@storybook/react";
import { FeatureBadge } from "./FeatureBadge";

const meta: Meta<typeof FeatureBadge> = {
	title: "components/FeatureBadge",
	component: FeatureBadge,
	args: {
		type: "beta",
	},
};

export default meta;
type Story = StoryObj<typeof FeatureBadge>;

export const SmallInteractive: Story = {
	args: {
		size: "sm",
		variant: "interactive",
	},
};
