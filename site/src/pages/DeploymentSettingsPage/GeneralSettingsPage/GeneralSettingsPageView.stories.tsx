import type { Meta, StoryObj } from "@storybook/react";
import { MockDeploymentDAUResponse, mockApiError } from "testHelpers/entities";
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
		deploymentDAUs: MockDeploymentDAUResponse,
		invalidExperiments: [],
		safeExperiments: [],
		userStatusCountsOverTime: {
			status_counts: {
				active: [
					{
						date: "1/1/2024",
						count: 1,
					},
					{
						date: "1/2/2024",
						count: 8,
					},
					{
						date: "1/3/2024",
						count: 8,
					},
					{
						date: "1/4/2024",
						count: 6,
					},
					{
						date: "1/5/2024",
						count: 6,
					},
					{
						date: "1/6/2024",
						count: 6,
					},
					{
						date: "1/7/2024",
						count: 6,
					},
				],
				dormant: [
					{
						date: "1/1/2024",
						count: 0,
					},
					{
						date: "1/2/2024",
						count: 3,
					},
					{
						date: "1/3/2024",
						count: 3,
					},
					{
						date: "1/4/2024",
						count: 3,
					},
					{
						date: "1/5/2024",
						count: 3,
					},
					{
						date: "1/6/2024",
						count: 3,
					},
					{
						date: "1/7/2024",
						count: 3,
					},
				],
				suspended: [
					{
						date: "1/1/2024",
						count: 0,
					},
					{
						date: "1/2/2024",
						count: 0,
					},
					{
						date: "1/3/2024",
						count: 0,
					},
					{
						date: "1/4/2024",
						count: 2,
					},
					{
						date: "1/5/2024",
						count: 2,
					},
					{
						date: "1/6/2024",
						count: 2,
					},
					{
						date: "1/7/2024",
						count: 2,
					},
				],
			},
		},
	},
};

export default meta;
type Story = StoryObj<typeof GeneralSettingsPageView>;

export const Page: Story = {};

export const NoDAUs: Story = {
	args: {
		deploymentDAUs: undefined,
	},
};

export const DAUError: Story = {
	args: {
		deploymentDAUs: undefined,
		deploymentDAUsError: mockApiError({
			message: "Error fetching DAUs.",
		}),
	},
};

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

export const UnlicensedInstallation: Story = {
	args: {},
};

export const LicensedWithNoUserLimit: Story = {
	args: {},
};

export const LicensedWithPlentyOfSpareLicenses: Story = {
	args: {
		activeUserLimit: 100,
	},
};

export const TotalUsersExceedsLicenseButNotActiveUsers: Story = {
	args: {
		activeUserLimit: 8,
	},
};

export const ManyUsers: Story = {
	args: {},
};
