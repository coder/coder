import type { Meta, StoryObj } from "@storybook/react";
import { Settings, TriangleAlert } from "lucide-react";
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

export const Warning: Story = {
	args: {
		variant: "warning",
	},
};

export const SmallWithIcon: Story = {
	args: {
		variant: "default",
		size: "sm",
		children: <>{<Settings />} Preset</>,
	},
};

export const MediumWithIcon: Story = {
	args: {
		variant: "warning",
		size: "md",
		children: <>{<TriangleAlert />} Immutable</>,
	},
};
