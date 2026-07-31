import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { MockOAuth2ProviderApps } from "#/testHelpers/entities";
import OAuth2AppsSettingsPageView from "./OAuth2AppsSettingsPageView";

const meta: Meta<typeof OAuth2AppsSettingsPageView> = {
	title: "pages/DeploymentSettingsPage/OAuth2AppsSettingsPageView",
	component: OAuth2AppsSettingsPageView,
	args: {
		canCreateApp: true,
		canViewSettings: true,
		canEditSettings: true,
		settingsError: undefined,
		isLoadingSettings: false,
		isUpdatingSettings: false,
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
		canViewSettings: false,
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
 * A settings failure is scoped to the settings tab. The applications empty
 * state still renders, because the apps request succeeded and only the `error`
 * prop gates that message.
 */
export const SettingsFetchErrorKeepsAppsEmptyState: Story = {
	args: {
		isLoading: false,
		apps: [],
		settingsError: "settings boom",
		dynamicClientRegistrationEnabled: undefined,
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
		isLoading: false,
		apps: MockOAuth2ProviderApps,
		settingsError: "update boom",
		dynamicClientRegistrationEnabled: false,
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
		isLoading: false,
		dynamicClientRegistrationEnabled: undefined,
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

// An unpermitted deep link selects the applications tab rather than leaving no
// tab selected at all.
export const UnpermittedTabFromUrlFallsBack: Story = {
	args: {
		canViewSettings: false,
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
	},
};

// The tab is present from first paint so it does not shift into the tab bar
// once the settings request resolves.
export const SettingsTabLoading: Story = {
	args: {
		isLoadingSettings: true,
		dynamicClientRegistrationEnabled: undefined,
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
