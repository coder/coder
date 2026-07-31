import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { MockOAuth2ProviderApps } from "#/testHelpers/entities";
import OAuth2AppsSettingsPageView from "./OAuth2AppsSettingsPageView";

// Spread and override per story. Omitting `settings` entirely is how a viewer
// without deployment config read access is expressed.
const MockSettingsTab = {
	canEdit: true,
	isLoading: false,
	isUpdating: false,
	error: undefined,
	dynamicClientRegistrationEnabled: false,
	onDynamicClientRegistrationChange: fn(),
};

const meta: Meta<typeof OAuth2AppsSettingsPageView> = {
	title: "pages/DeploymentSettingsPage/OAuth2AppsSettingsPageView",
	component: OAuth2AppsSettingsPageView,
	args: {
		canCreateApp: true,
		settings: MockSettingsTab,
	},
};
export default meta;

type Story = StoryObj<typeof OAuth2AppsSettingsPageView>;

export const Loading: Story = {
	args: {
		isLoadingApps: true,
	},
};

export const WithError: Story = {
	args: {
		isLoadingApps: false,
		error: "some error",
	},
};

export const Apps: Story = {
	args: {
		isLoadingApps: false,
		apps: MockOAuth2ProviderApps,
	},
};

export const Empty: Story = {
	args: {
		isLoadingApps: false,
	},
};

export const NoCreatePermissions: Story = {
	args: {
		canCreateApp: false,
	},
};

// Setting behavior is covered in DynamicClientRegistrationSetting.stories.tsx;
// this covers only the wiring.
export const SettingsTabWiresDynamicClientRegistration: Story = {
	args: {
		settings: { ...MockSettingsTab, dynamicClientRegistrationEnabled: true },
	},
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("tab", { name: "Settings" }));

		await userEvent.click(canvas.getByRole("button", { name: "Disable" }));
		await expect(
			args.settings?.onDynamicClientRegistrationChange,
		).toHaveBeenCalledWith(false);
	},
};

export const SettingsTabHiddenWithoutPermission: Story = {
	args: {
		settings: undefined,
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

/**
 * The header sits outside the tabs, so its action is scoped to the tab it
 * belongs to. Offering "Add application" while the settings tab is open would
 * promise to act on the settings below it and then navigate away.
 */
export const AddApplicationIsScopedToApplicationsTab: Story = {
	args: {
		isLoadingApps: false,
		apps: MockOAuth2ProviderApps,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const header = canvas.getByRole("link", { name: "Add application" });
		await expect(header).toBeVisible();

		await userEvent.click(canvas.getByRole("tab", { name: "Settings" }));
		await expect(
			canvas.queryByRole("link", { name: "Add application" }),
		).not.toBeInTheDocument();

		await userEvent.click(canvas.getByRole("tab", { name: "Applications" }));
		await expect(
			canvas.getByRole("link", { name: "Add application" }),
		).toBeVisible();
	},
};

/**
 * A settings failure is scoped to the settings tab. The applications empty
 * state still renders, because the apps request succeeded and only the `error`
 * prop gates that message.
 */
export const SettingsFetchErrorKeepsAppsEmptyState: Story = {
	args: {
		isLoadingApps: false,
		apps: [],
		settings: {
			...MockSettingsTab,
			error: "settings boom",
			dynamicClientRegistrationEnabled: undefined,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await expect(
			canvas.getByText("No OAuth2 applications configured"),
		).toBeVisible();

		await userEvent.click(canvas.getByRole("tab", { name: "Settings" }));
		await expect(canvas.getByText("settings boom")).toBeVisible();
		await expect(
			canvas.queryByRole("button", { name: "Enable" }),
		).not.toBeInTheDocument();
	},
};

/**
 * A failed update leaves the setting on screen with the error above it, so the
 * admin can see the current value and retry.
 */
export const SettingsUpdateErrorKeepsSettingVisible: Story = {
	args: {
		isLoadingApps: false,
		apps: MockOAuth2ProviderApps,
		settings: { ...MockSettingsTab, error: "update boom" },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("tab", { name: "Settings" }));

		await expect(canvas.getByText("update boom")).toBeVisible();
		await expect(canvas.getByRole("button", { name: "Enable" })).toBeVisible();
	},
};

/**
 * The value is optional on the wire. A response that omits it must not leave
 * the tab silently blank.
 */
export const SettingsValueOmitted: Story = {
	args: {
		isLoadingApps: false,
		settings: {
			...MockSettingsTab,
			dynamicClientRegistrationEnabled: undefined,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("tab", { name: "Settings" }));

		await expect(canvas.getByText("Settings are unavailable.")).toBeVisible();
	},
};

export const SettingsTabFromUrl: Story = {
	parameters: {
		reactRouter: reactRouterParameters({
			location: { searchParams: { tab: "settings" } },
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await expect(canvas.getByRole("tab", { name: "Settings" })).toHaveAttribute(
			"aria-selected",
			"true",
		);
		await expect(canvas.getByRole("button", { name: "Enable" })).toBeVisible();
	},
};

export const UnpermittedTabFromUrlFallsBack: Story = {
	args: {
		settings: undefined,
	},
	parameters: {
		reactRouter: reactRouterParameters({
			location: { searchParams: { tab: "settings" } },
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await expect(
			canvas.getByRole("tab", { name: "Applications" }),
		).toHaveAttribute("aria-selected", "true");
		// Which tab is highlighted says nothing about what rendered. Radix mounts
		// no inactive TabsContent today, so a later forceMount would otherwise
		// hand this user a control the deep link should not have reached.
		await expect(
			canvas.queryByRole("button", { name: "Enable" }),
		).not.toBeInTheDocument();
	},
};

// So the tab does not pop into the tab bar when the request resolves.
export const SettingsTabLoading: Story = {
	args: {
		settings: {
			...MockSettingsTab,
			isLoading: true,
			dynamicClientRegistrationEnabled: undefined,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByRole("tab", { name: "Settings" })).toBeVisible();

		await userEvent.click(canvas.getByRole("tab", { name: "Settings" }));

		await expect(canvas.getByLabelText("Loading settings")).toBeVisible();
		await expect(
			canvas.queryByRole("button", { name: "Enable" }),
		).not.toBeInTheDocument();
	},
};
