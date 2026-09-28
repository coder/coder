import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, spyOn, userEvent, within } from "storybook/test";
import { API } from "#/api/api";
import { externalScopesKey } from "#/api/queries/oauth2";
import type * as TypesGen from "#/api/typesGenerated";
import {
	MockExternalAPIKeyScopes,
	MockOAuth2ProviderAppDynamic,
	MockOAuth2ProviderApps,
	mockApiError,
} from "#/testHelpers/entities";
import { OAuth2AppForm } from "./OAuth2AppForm";

const meta = {
	title: "pages/DeploymentSettingsPage/OAuth2AppForm",
	component: OAuth2AppForm,
	args: {
		clientType: "confidential",
		onSubmit: fn(),
		isUpdating: false,
		disabled: false,
	},
	parameters: {
		queries: [{ key: externalScopesKey, data: MockExternalAPIKeyScopes }],
	},
} satisfies Meta<typeof OAuth2AppForm>;

export default meta;
type Story = StoryObj<typeof OAuth2AppForm>;

const appWithScopes: TypesGen.OAuth2ProviderApp = {
	...MockOAuth2ProviderApps[0],
	scope: "coder:workspaces.access workspace:ssh",
};

export const Default: Story = {};

export const WithScopes: Story = {
	args: { app: appWithScopes },
};

export const ScopesOpen: Story = {
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("combobox", { name: /allowed scopes/i }),
		);
	},
};

export const ScopeCatalogLoading: Story = {
	parameters: { queries: [] },
	beforeEach: () => {
		spyOn(API, "getExternalAPIKeyScopes").mockReturnValue(
			new Promise(() => {}),
		);
	},
};

export const ScopeCatalogError: Story = {
	parameters: { queries: [] },
	beforeEach: () => {
		spyOn(API, "getExternalAPIKeyScopes").mockRejectedValue(
			mockApiError({ message: "Failed to load the list of scopes." }),
		);
	},
};

// Existing selections must not hide the loading message while the catalog
// request is pending.
export const ConfiguredScopesWithCatalogLoading: Story = {
	args: { app: appWithScopes },
	parameters: { queries: [] },
	beforeEach: () => {
		spyOn(API, "getExternalAPIKeyScopes").mockReturnValue(
			new Promise(() => {}),
		);
	},
};

// An unreachable catalog must not hide an allowlist the app already has.
export const ConfiguredScopesWithCatalogError: Story = {
	args: { app: appWithScopes },
	parameters: { queries: [] },
	beforeEach: () => {
		spyOn(API, "getExternalAPIKeyScopes").mockRejectedValue(
			mockApiError({ message: "Failed to load the list of scopes." }),
		);
	},
};

export const Disabled: Story = {
	args: {
		app: appWithScopes,
		disabled: true,
	},
};

export const SelfRegisteredScopesNarrowed: Story = {
	args: {
		app: {
			...MockOAuth2ProviderAppDynamic,
			scope: "coder:workspaces.access workspace:ssh",
		},
	},
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getAllByTestId("clear-option-button")[0],
		);
	},
};

export const AdminCreatedScopesNarrowed: Story = {
	args: { app: appWithScopes },
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getAllByTestId("clear-option-button")[0],
		);
	},
};

export const SelfRegisteredScopesWidened: Story = {
	args: {
		app: {
			...MockOAuth2ProviderAppDynamic,
			scope: "coder:workspaces.access workspace:ssh",
		},
	},
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("combobox", { name: /allowed scopes/i }),
		);
		await userEvent.click(
			await within(canvasElement).findByRole("option", {
				name: "workspace:read",
			}),
		);
	},
};
