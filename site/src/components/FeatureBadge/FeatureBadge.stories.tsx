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
