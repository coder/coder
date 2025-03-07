import type { Meta, StoryObj } from "@storybook/react";
import { MockDeploymentDAUResponse } from "testHelpers/entities";
import { GeneralSettingsPageView } from "./GeneralSettingsPageView";

const meta: Meta<typeof GeneralSettingsPageView> = {
	title: "pages/DeploymentSettingsPage/GeneralSettingsPageView",
	component: GeneralSettingsPageView,
	args: {
		deploymentOptions: [
			{
				name: "Access URL",
				description:
					"The URL that users will use to access the Coder deployment.",
				flag: "access-url",
				flag_shorthand: "",
				value: "https://dev.coder.com",
				hidden: false,
			},
			{
				name: "Wildcard Access URL",
				description:
					'Specifies the wildcard hostname to use for workspace applications in the form "*.example.com".',
				flag: "wildcard-access-url",
				flag_shorthand: "",
				value: "*--apps.dev.coder.com",
				hidden: false,
			},
			{
				name: "Experiments",
				description:
					"Enable one or more experiments. These are not ready for production. Separate multiple experiments with commas, or enter '*' to opt-in to all available experiments.",
				flag: "experiments",
				value: ["workspace_actions"],
				flag_shorthand: "",
				hidden: false,
			},
		],
		dailyActiveUsers: MockDeploymentDAUResponse,
		invalidExperiments: [],
		safeExperiments: [],
	},
};

export default meta;
type Story = StoryObj<typeof GeneralSettingsPageView>;

export const Page: Story = {};

export const allExperimentsEnabled: Story = {
	args: {
		deploymentOptions: [
			{
				name: "Access URL",
				description:
					"The URL that users will use to access the Coder deployment.",
				flag: "access-url",
				flag_shorthand: "",
				value: "https://dev.coder.com",
				hidden: false,
			},
			{
				name: "Wildcard Access URL",
				description:
					'Specifies the wildcard hostname to use for workspace applications in the form "*.example.com".',
				flag: "wildcard-access-url",
				flag_shorthand: "",
				value: "*--apps.dev.coder.com",
				hidden: false,
			},
			{
				name: "Experiments",
				description:
					"Enable one or more experiments. These are not ready for production. Separate multiple experiments with commas, or enter '*' to opt-in to all available experiments.",
				flag: "experiments",
				value: ["*"],
				flag_shorthand: "",
				hidden: false,
			},
		],
		safeExperiments: ["shared-ports"],
		invalidExperiments: ["invalid"],
	},
};

export const invalidExperimentsEnabled: Story = {
	args: {
		deploymentOptions: [
			{
				name: "Access URL",
				description:
					"The URL that users will use to access the Coder deployment.",
				flag: "access-url",
				flag_shorthand: "",
				value: "https://dev.coder.com",
				hidden: false,
			},
			{
				name: "Wildcard Access URL",
				description:
					'Specifies the wildcard hostname to use for workspace applications in the form "*.example.com".',
				flag: "wildcard-access-url",
				flag_shorthand: "",
				value: "*--apps.dev.coder.com",
				hidden: false,
			},
			{
				name: "Experiments",
				description:
					"Enable one or more experiments. These are not ready for production. Separate multiple experiments with commas, or enter '*' to opt-in to all available experiments.",
				flag: "experiments",
				value: ["invalid", "*"],
				flag_shorthand: "",
				hidden: false,
			},
		],
		safeExperiments: ["shared-ports"],
		invalidExperiments: ["invalid"],
	},
};
