import type { Meta, StoryObj } from "@storybook/react";
import {
	MockDeploymentDAUResponse,
	MockEntitlementsWithUserLimit,
	mockApiError,
} from "testHelpers/entities";
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
		entitlements: undefined,
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

export const WithLicenseUtilization: Story = {
	args: {
		entitlements: {
			...MockEntitlementsWithUserLimit,
			features: {
				...MockEntitlementsWithUserLimit.features,
				user_limit: {
					...MockEntitlementsWithUserLimit.features.user_limit,
					enabled: true,
					actual: 75,
					limit: 100,
					entitlement: "entitled",
				},
			},
		},
	},
};

export const HighLicenseUtilization: Story = {
	args: {
		entitlements: {
			...MockEntitlementsWithUserLimit,
			features: {
				...MockEntitlementsWithUserLimit.features,
				user_limit: {
					...MockEntitlementsWithUserLimit.features.user_limit,
					enabled: true,
					actual: 95,
					limit: 100,
					entitlement: "entitled",
				},
			},
		},
	},
};

export const ExceedsLicenseUtilization: Story = {
	args: {
		entitlements: {
			...MockEntitlementsWithUserLimit,
			features: {
				...MockEntitlementsWithUserLimit.features,
				user_limit: {
					...MockEntitlementsWithUserLimit.features.user_limit,
					enabled: true,
					actual: 100,
					limit: 95,
					entitlement: "entitled",
				},
			},
		},
	},
};
export const NoLicenseLimit: Story = {
	args: {
		entitlements: {
			...MockEntitlementsWithUserLimit,
			features: {
				...MockEntitlementsWithUserLimit.features,
				user_limit: {
					...MockEntitlementsWithUserLimit.features.user_limit,
					enabled: false,
					actual: 0,
					limit: 0,
					entitlement: "entitled",
				},
			},
		},
	},
};
