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

// The setting's own behavior is covered by
// DynamicClientRegistrationSetting.stories.tsx. This story covers the wiring
// between the two: rendering "Disable" proves the enabled state is threaded
// through, and clicking it proves the change handler is connected.
export const SettingsTabRendersDynamicClientRegistration: Story = {
	args: {
		dynamicClientRegistrationEnabled: true,
	},
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("tab", { name: "Settings" }));

		await userEvent.click(canvas.getByRole("button", { name: "Disable" }));
		await expect(args.onDynamicClientRegistrationChange).toHaveBeenCalledWith(
			false,
		);
	},
};

export const SettingsTabHiddenWithoutPermission: Story = {
	args: {
		dynamicClientRegistrationEnabled: undefined,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await expect(
			canvas.getByRole("tab", { name: "Applications" }),
		).toBeVisible();
		await expect(
			canvas.queryByRole("tab", { name: "Settings" }),
		).not.toBeInTheDocument();
	},
};
