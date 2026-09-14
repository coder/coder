import type { Meta, StoryObj } from "@storybook/react-vite";
import { AIBridgeSetupAlert } from "./AIBridgeSetupAlert";

const meta: Meta<typeof AIBridgeSetupAlert> = {
	title: "pages/AIBridgePage/AIBridgeSetupAlert",
	component: AIBridgeSetupAlert,
};

export default meta;
type Story = StoryObj<typeof AIBridgeSetupAlert>;

export const Alert: Story = {
	args: {},
};
