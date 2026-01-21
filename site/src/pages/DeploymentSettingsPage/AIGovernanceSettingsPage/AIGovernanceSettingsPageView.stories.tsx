import type { Meta, StoryObj } from "@storybook/react-vite";
import { AIGovernanceSettingsPageView } from "./AIGovernanceSettingsPageView";

const meta: Meta<typeof AIGovernanceSettingsPageView> = {
	title: "pages/DeploymentSettingsPage/AIGovernanceSettingsPageView",
	component: AIGovernanceSettingsPageView,
	args: {
		options: [
			{
				name: "AI Bridge Enabled",
				value: true,
				group: { name: "AI Bridge" },
				flag: "aibridge-enabled",
				hidden: false,
			},
			{
				name: "AI Bridge Circuit Breaker Enabled",
				description:
					"Enable the circuit breaker to protect against cascading failures from upstream AI provider rate limits.",
				value: false,
				group: { name: "AI Bridge" },
				flag: "aibridge-circuit-breaker-enabled",
				hidden: false,
			},
		],
		featureAIBridgeEnabled: true,
	},
};

export default meta;
type Story = StoryObj<typeof AIGovernanceSettingsPageView>;

export const Page: Story = {};

export const AIBridgeDisabled: Story = {
	args: {
		featureAIBridgeEnabled: false,
		options: [],
	},
};
