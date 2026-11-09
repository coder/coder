import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { ChimeButton } from "./ChimeButton";

const meta: Meta<typeof ChimeButton> = {
	title: "pages/AgentsPage/ChimeButton",
	component: ChimeButton,
	args: {
		enabled: false,
		onToggle: fn(),
	},
};
export default meta;
type Story = StoryObj<typeof ChimeButton>;

export const Default: Story = {};

export const Enabled: Story = {
	args: {
		enabled: true,
	},
};
