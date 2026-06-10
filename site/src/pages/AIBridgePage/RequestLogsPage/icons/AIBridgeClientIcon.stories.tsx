import type { Meta, StoryObj } from "@storybook/react-vite";
import { AIBridgeClientIcon } from "./AIBridgeClientIcon";

const meta: Meta<typeof AIBridgeClientIcon> = {
	title: "pages/AIBridgePage/AIBridgeClientIcon",
	component: AIBridgeClientIcon,
	args: {
		className: "size-8",
	},
};

export default meta;
type Story = StoryObj<typeof AIBridgeClientIcon>;

export const OpenCode: Story = {
	args: {
		client: "OpenCode",
	},
};

export const ClaudeCode: Story = {
	args: {
		client: "Claude Code",
	},
};

export const Unknown: Story = {
	args: {
		client: "Unknown",
	},
};
