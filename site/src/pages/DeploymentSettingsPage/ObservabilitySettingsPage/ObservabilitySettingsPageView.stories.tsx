import type { Meta, StoryObj } from "@storybook/react-vite";
import type { SerpentGroup } from "api/typesGenerated";
import { ObservabilitySettingsPageView } from "./ObservabilitySettingsPageView";

const group: SerpentGroup = {
	name: "Introspection",
	description: "",
};

const meta: Meta<typeof ObservabilitySettingsPageView> = {
	title: "pages/DeploymentSettingsPage/ObservabilitySettingsPageView",
	component: ObservabilitySettingsPageView,
	args: {
		options: [
			{
				name: "Verbose",
				value: true,
				group,
				flag: "verbose",
				flag_shorthand: "v",
				hidden: false,
			},
			{
				name: "Human Log Location",
				description: "Output human-readable logs to a given file.",
				value: "/dev/stderr",
				flag: "log-human",
				hidden: false,
			},
			{
				name: "Stackdriver Log Location",
				description: "Output Stackdriver compatible logs to a given file.",
				value: "",
				flag: "log-stackdriver",
				hidden: false,
			},
			{
				name: "Prometheus Enable",
				description:
					"Serve prometheus metrics on the address defined by prometheus address.",
				value: true,
				group: { ...group },
				flag: "prometheus-enable",
				hidden: false,
			},
		],
		featureAuditLogEnabled: true,
	},
};

export default meta;
type Story = StoryObj<typeof ObservabilitySettingsPageView>;

export const Page: Story = {};

export const Premium: Story = { args: { isPremium: true } };

export const AIBridge: Story = {
	args: {
		featureAIBridgeEnabled: true,
		options: [
			{
				name: "AI Bridge Enabled",
				value: true,
				group: { name: "AI Bridge" },
				flag: "aibridge-enabled",
				hidden: false,
			},
		],
	},
};
