import type { Meta, StoryObj } from "@storybook/react-vite";
import { AIGovernanceSettingsPageView } from "./AIGovernanceSettingsPageView";

const meta: Meta<typeof AIGovernanceSettingsPageView> = {
	title: "pages/DeploymentSettingsPage/AIGovernanceSettingsPageView",
	component: AIGovernanceSettingsPageView,
	args: {
		options: [
			{
				name: "AI Gateway Enabled",
				value: true,
				group: { name: "AI Gateway" },
				flag: "ai-gateway-enabled",
				hidden: false,
			},
			{
				name: "AI Gateway Circuit Breaker Enabled",
				description:
					"Enable the circuit breaker to protect against cascading failures from upstream AI provider rate limits.",
				value: false,
				group: { name: "AI Gateway" },
				flag: "ai-gateway-circuit-breaker-enabled",
				hidden: false,
			},
		],
		featureAIBridgeEntitled: true,
	},
};

export default meta;
type Story = StoryObj<typeof AIGovernanceSettingsPageView>;

export const Page: Story = {};

export const Paywall: Story = {
	args: {
		featureAIBridgeEntitled: false,
		options: [],
	},
};
