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

export const DynamicClientRegistrationDisabled: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("tab", { name: "Settings" }));

		await expect(canvas.getByRole("button", { name: "Enable" })).toBeEnabled();
		await expect(canvas.queryByText("Enabled")).not.toBeInTheDocument();
	},
};

export const DynamicClientRegistrationEnabled: Story = {
	args: {
		dynamicClientRegistrationEnabled: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("tab", { name: "Settings" }));

		await expect(canvas.getByText("Enabled")).toBeVisible();
		await expect(canvas.getByRole("button", { name: "Disable" })).toBeVisible();
	},
};

export const DynamicClientRegistrationReadOnly: Story = {
	args: {
		canEditSettings: false,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("tab", { name: "Settings" }));

		await expect(canvas.getByRole("button", { name: "Enable" })).toBeDisabled();
	},
};

export const EnableDynamicClientRegistrationDialog: Story = {
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("tab", { name: "Settings" }));
		await userEvent.click(canvas.getByRole("button", { name: "Enable" }));

		const body = within(canvasElement.ownerDocument.body);
		await body.findByText("Enable Dynamic Client Registration?");
		await expect(args.onDynamicClientRegistrationChange).not.toHaveBeenCalled();

		await userEvent.click(body.getByRole("button", { name: "Confirm" }));
		await expect(args.onDynamicClientRegistrationChange).toHaveBeenCalledWith(
			true,
		);
	},
};

export const CancelEnableDynamicClientRegistration: Story = {
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("tab", { name: "Settings" }));
		await userEvent.click(canvas.getByRole("button", { name: "Enable" }));

		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(body.getByRole("button", { name: "Cancel" }));

		await expect(args.onDynamicClientRegistrationChange).not.toHaveBeenCalled();
	},
};

// Disabling skips the confirmation dialog, unlike enabling.
export const DisableDynamicClientRegistration: Story = {
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
