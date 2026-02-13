import {
	MockOAuth2ProviderAppSecrets,
	MockOAuth2ProviderApps,
	mockApiError,
} from "testHelpers/entities";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { EditOAuth2AppPageView } from "./EditOAuth2AppPageView";

const meta: Meta = {
	title: "pages/DeploymentSettingsPage/EditOAuth2AppPageView",
	component: EditOAuth2AppPageView,
	args: {
		canEditApp: true,
		canDeleteApp: true,
		canViewAppSecrets: true,
	},
};
export default meta;

type Story = StoryObj<typeof EditOAuth2AppPageView>;

export const LoadingApp: Story = {
	args: {
		isLoadingApp: true,
		mutatingResource: {
			updateApp: false,
			deleteApp: false,
			createSecret: false,
			deleteSecret: false,
		},
	},
};

export const LoadingSecrets: Story = {
	args: {
		app: MockOAuth2ProviderApps[0],
		isLoadingSecrets: true,
		mutatingResource: {
			updateApp: false,
			deleteApp: false,
			createSecret: false,
			deleteSecret: false,
		},
	},
};

export const WithError: Story = {
	args: {
		app: MockOAuth2ProviderApps[0],
		secrets: MockOAuth2ProviderAppSecrets,
		mutatingResource: {
			updateApp: false,
			deleteApp: false,
			createSecret: false,
			deleteSecret: false,
		},
		error: mockApiError({
			message: "Validation failed",
			validations: [
				{
					field: "name",
					detail: "name error",
				},
				{
					field: "callback_url",
					detail: "url error",
				},
				{
					field: "icon",
					detail: "icon error",
				},
			],
		}),
	},
};

export const Default: Story = {
	args: {
		app: MockOAuth2ProviderApps[0],
		secrets: MockOAuth2ProviderAppSecrets,
		mutatingResource: {
			updateApp: false,
			deleteApp: false,
			createSecret: false,
			deleteSecret: false,
		},
	},
};
