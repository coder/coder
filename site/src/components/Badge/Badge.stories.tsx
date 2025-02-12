import type { Meta, StoryObj } from "@storybook/react";
import { Badge } from "./Badge";

const meta: Meta<typeof Badge> = {
	title: "components/Badge",
	component: Badge,
	args: {
		children: "Badge",
	},
};

export default meta;
type Story = StoryObj<typeof Badge>;

export const Default: Story = {};
