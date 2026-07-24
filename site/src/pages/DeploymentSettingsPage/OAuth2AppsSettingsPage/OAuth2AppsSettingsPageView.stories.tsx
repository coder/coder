import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import { MockOAuth2ProviderApps } from "#/testHelpers/entities";
import OAuth2AppsSettingsPageView from "./OAuth2AppsSettingsPageView";

const meta: Meta<typeof OAuth2AppsSettingsPageView> = {
	title: "pages/DeploymentSettingsPage/OAuth2AppsSettingsPageView",
	component: OAuth2AppsSettingsPageView,
	args: {
		canCreateApp: true,
		canEditSettings: true,
		dynamicClientRegistrationEnabled: false,
		onDynamicClientRegistrationChange: fn(),
	},
};
export default meta;

type Story = StoryObj<typeof OAuth2AppsSettingsPageView>;

export const Loading: Story = {
	args: {
		isLoading: true,
	},
};

export const WithError: Story = {
	args: {
		isLoading: false,
		error: "some error",
	},
};

export const Apps: Story = {
	args: {
		isLoading: false,
		apps: MockOAuth2ProviderApps,
	},
};

export const Empty: Story = {
	args: {
		isLoading: false,
	},
};

export const NoCreatePermissions: Story = {
	args: {
		canCreateApp: false,
	},
};

export const DynamicClientRegistrationEnabled: Story = {
	args: {
		dynamicClientRegistrationEnabled: true,
	},
};

export const DynamicClientRegistrationReadOnly: Story = {
	args: {
		canEditSettings: false,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const dcrSwitch = canvas.getByRole("switch", {
			name: "Dynamic Client Registration",
		});
		expect(dcrSwitch).toBeDisabled();
	},
};

export const EnableDynamicClientRegistrationDialog: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("switch", { name: "Dynamic Client Registration" }),
		);
		const body = within(canvasElement.ownerDocument.body);
		await body.findByText("Enable Dynamic Client Registration");
	},
};
