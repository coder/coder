import type { Meta, StoryObj } from "@storybook/react-vite";
import { PaywallAIGovernance } from "./PaywallAIGovernance";

const meta: Meta<typeof PaywallAIGovernance> = {
	title: "components/Paywall/AIGovernance",
	component: PaywallAIGovernance,
};

export default meta;
type Story = StoryObj<typeof PaywallAIGovernance>;

export const Default: Story = {};
