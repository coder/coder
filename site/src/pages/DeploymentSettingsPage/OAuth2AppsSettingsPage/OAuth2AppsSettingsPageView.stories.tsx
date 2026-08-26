import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import type { OAuth2ProviderAppsResponse } from "#/api/typesGenerated";
import type { UseFilterResult } from "#/components/Filter/Filter";
import type { PaginationResult } from "#/components/PaginationWidget/PaginationContainer";
import {
	mockInitialRenderResult,
	mockSuccessResult,
} from "#/components/PaginationWidget/PaginationContainer.mocks";
import { MockOAuth2ProviderApps } from "#/testHelpers/entities";
import OAuth2AppsSettingsPageView from "./OAuth2AppsSettingsPageView";

const defaultFilter: UseFilterResult = {
	query: "",
	values: {},
	update: () => {},
	debounceUpdate: () => {},
	cancelDebounce: () => {},
	used: false,
};

const appsQuerySuccess: PaginationResult<OAuth2ProviderAppsResponse> = {
	...mockSuccessResult,
	totalRecords: MockOAuth2ProviderApps.length,
	data: {
		apps: MockOAuth2ProviderApps,
		count: MockOAuth2ProviderApps.length,
	},
};

// Spread and override per story. Omitting `settings` entirely is how a viewer
// without deployment config read access is expressed.
const MockSettingsTab = {
	canEdit: true,
	isLoading: false,
	isUpdating: false,
	loadError: undefined,
	updateError: undefined,
	onRetry: fn(),
	dynamicClientRegistrationEnabled: false,
	onDynamicClientRegistrationChange: fn(),
};

const meta: Meta<typeof OAuth2AppsSettingsPageView> = {
	title: "pages/DeploymentSettingsPage/OAuth2AppsSettingsPageView",
	component: OAuth2AppsSettingsPageView,
	args: {
		canCreateApp: true,
		filter: defaultFilter,
		apps: MockOAuth2ProviderApps,
		appsQuery: appsQuerySuccess,
		isLoadingApps: false,
		appsError: undefined,
		settings: MockSettingsTab,
	},
};

export default meta;

type Story = StoryObj<typeof OAuth2AppsSettingsPageView>;

export const Loading: Story = {
	args: {
		isLoadingApps: true,
		apps: undefined,
		appsQuery: {
			...mockInitialRenderResult,
			data: undefined,
		},
	},
};

/**
 * An apps failure belongs to the applications tab. Rendered above the tabs it
 * would sit over a settings panel that loaded fine, at the moment the admin is
 * deciding whether to open self-registration.
 */
export const WithError: Story = {
	args: {
		isLoadingApps: false,
		apps: undefined,
		appsError: "some error",
		appsQuery: {
			...mockInitialRenderResult,
			data: undefined,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("some error")).toBeVisible();

		await userEvent.click(canvas.getByRole("tab", { name: "Settings" }));
		await expect(canvas.queryByText("some error")).not.toBeInTheDocument();

		await userEvent.click(canvas.getByRole("tab", { name: "Applications" }));
		await expect(canvas.getByText("some error")).toBeVisible();
	},
};

// Rendering assertions are left to Storybook snapshots; only stories that
// exercise real interactions keep a play function.
export const Apps: Story = {};

export const Empty: Story = {
	args: {
		isLoadingApps: false,
		apps: [],
		appsQuery: {
			...mockSuccessResult,
			totalRecords: 0,
			data: {
				apps: [],
				count: 0,
			},
		},
	},
};

export const EmptySearch: Story = {
	args: {
		isLoadingApps: false,
		apps: [],
		filter: {
			...defaultFilter,
			query: "nonexistent",
			used: true,
		},
		appsQuery: {
			...mockSuccessResult,
			totalRecords: 0,
			data: {
				apps: [],
				count: 0,
			},
		},
	},
};

export const NoCreatePermissions: Story = {
	args: {
		canCreateApp: false,
		apps: [],
		appsQuery: {
			...mockSuccessResult,
			totalRecords: 0,
			data: {
				apps: [],
				count: 0,
			},
		},
	},
};

export const Paginated: Story = {
	args: {
		apps: MockOAuth2ProviderApps.slice(0, 2),
		appsQuery: {
			...mockSuccessResult,
			currentPage: 1,
			totalPages: 2,
			totalRecords: 5,
			hasNextPage: true,
			hasPreviousPage: false,
			currentOffsetStart: 1,
			limit: 2,
			data: {
				apps: MockOAuth2ProviderApps.slice(0, 2),
				count: 5,
			},
		},
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

/**
 * An in-flight save reaches the control only through this view. Container
 * stories cover the permission props by rendering the whole page, but they
 * cannot hold a mutation pending, so the passthrough is pinned here.
 */
export const SettingsTabUpdating: Story = {
	args: {
		settings: { ...MockSettingsTab, isUpdating: true },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("tab", { name: "Settings" }));

		await expect(
			canvas.getByRole("button", { name: "Enable" }),
		).toHaveAttribute("aria-disabled", "true");
	},
};

/**
 * The description is header content, outside the tabs, so it has to describe
 * what the page actually offers this viewer. Its second clause belongs to the
 * settings tab and goes with it.
 */
export const DescriptionCoversSettingsWhenPermitted: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await expect(
			canvas.getByText(
				"Register applications to use Coder as an OAuth2 provider, and configure how this deployment behaves as one.",
			),
		).toBeVisible();
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
		await expect(
			canvas.getByText(
				"Register applications to use Coder as an OAuth2 provider.",
			),
		).toBeVisible();
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

		// The docs link is tab-agnostic and stays put, which is what makes the
		// other one's disappearance a scoping decision rather than a quirk.
		const docsLink = canvas.getByRole("link", { name: /read the docs/i });
		await expect(docsLink).toBeVisible();

		await userEvent.click(canvas.getByRole("tab", { name: "Settings" }));
		await expect(
			canvas.queryByRole("link", { name: "Add application" }),
		).not.toBeInTheDocument();
		await expect(
			canvas.getByRole("link", { name: /read the docs/i }),
		).toBeVisible();

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
			loadError: "settings boom",
			dynamicClientRegistrationEnabled: undefined,
		},
	},
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);

		await expect(
			canvas.getByText("No OAuth2 applications configured"),
		).toBeVisible();

		await userEvent.click(canvas.getByRole("tab", { name: "Settings" }));
		await expect(canvas.getByText("settings boom")).toBeVisible();
		await expect(
			canvas.queryByRole("button", { name: "Enable" }),
		).not.toBeInTheDocument();
		// One condition, one explanation. The fallback copy is for a value that is
		// absent without an error, not for an error.
		await expect(
			canvas.queryByText(/did not return a value/),
		).not.toBeInTheDocument();
		await userEvent.click(canvas.getByRole("button", { name: "Retry" }));
		await expect(args.settings?.onRetry).toHaveBeenCalled();
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
		settings: { ...MockSettingsTab, updateError: "update boom" },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("tab", { name: "Settings" }));

		await expect(canvas.getByText("update boom")).toBeVisible();
		await expect(canvas.getByRole("button", { name: "Enable" })).toBeVisible();
	},
};

/**
 * A load failure that left a usable value behind must not hide the failure of
 * the save the admin just attempted. This is the state a failed post-save
 * refetch produces, and the older error used to win it.
 */
export const UpdateErrorOutranksStaleLoadError: Story = {
	args: {
		isLoadingApps: false,
		apps: MockOAuth2ProviderApps,
		settings: {
			...MockSettingsTab,
			loadError: "stale refetch failure",
			updateError: "forbidden: your role changed",
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("tab", { name: "Settings" }));

		await expect(
			canvas.getByText("forbidden: your role changed"),
		).toBeVisible();
		await expect(
			canvas.queryByText("stale refetch failure"),
		).not.toBeInTheDocument();
		// The value is still valid, so the control stays and the admin can retry.
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
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("tab", { name: "Settings" }));

		// Names the cause and offers a way out. Nothing else recovers this state:
		// retries are off, refetch-on-focus is off, and the control that would
		// trigger an invalidation is the thing that is missing.
		await expect(
			canvas.getByText(
				/did not return a value for Dynamic Client Registration/,
			),
		).toBeVisible();
		await userEvent.click(canvas.getByRole("button", { name: "Retry" }));
		await expect(args.settings?.onRetry).toHaveBeenCalled();
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
