import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, spyOn, userEvent, waitFor, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { API } from "#/api/api";
import { externalScopesKey } from "#/api/queries/oauth2";
import {
	MockExternalAPIKeyScopes,
	MockPermissions,
	MockUserOwner,
	mockApiError,
} from "#/testHelpers/entities";
import { withAuthProvider, withToaster } from "#/testHelpers/storybook";
import { CreateOAuth2AppPageView } from "./CreateOAuth2AppPageView";

const meta = {
	title: "pages/DeploymentSettingsPage/CreateOAuth2AppPageView",
	component: CreateOAuth2AppPageView,
	parameters: {
		user: MockUserOwner,
		permissions: MockPermissions,
		queries: [{ key: externalScopesKey, data: MockExternalAPIKeyScopes }],
		reactRouter: reactRouterParameters({
			location: { path: "/deployment/oauth2-provider/apps/add" },
			routing: [
				{ path: "/deployment/oauth2-provider/apps", useStoryElement: true },
				{
					path: "/deployment/oauth2-provider/apps/add",
					useStoryElement: true,
				},
			],
		}),
	},
	decorators: [withToaster, withAuthProvider],
} satisfies Meta<typeof CreateOAuth2AppPageView>;

export default meta;
type Story = StoryObj<typeof CreateOAuth2AppPageView>;

export const Default: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			await canvas.findByRole("heading", {
				name: /add an oauth2 application/i,
			}),
		).toBeVisible();
		// The submit button renders enabled for a frame until the form's
		// validate-on-mount pass reports the empty required fields.
		await waitFor(() =>
			expect(
				canvas.getByRole("button", { name: /create application/i }),
			).toBeDisabled(),
		);
	},
};

export const WithValidationError: Story = {
	beforeEach: () => {
		spyOn(API, "postOAuth2ProviderApp").mockRejectedValue(
			mockApiError({
				message: "Validation failed",
				validations: [
					{ field: "name", detail: "name error" },
					{ field: "redirect_uris", detail: "url error" },
					{ field: "icon", detail: "icon error" },
				],
			}),
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.type(await canvas.findByLabelText(/^name/i), "test-app");
		await userEvent.type(
			canvas.getByLabelText(/default callback/i),
			"https://example.com/callback",
		);
		await userEvent.click(
			canvas.getByRole("button", { name: /create application/i }),
		);
		await expect(await canvas.findByText("name error")).toBeVisible();
		await expect(canvas.getByText("url error")).toBeVisible();
		await expect(canvas.getByText("icon error")).toBeVisible();
	},
};

export const InvalidCallbackURL: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.type(await canvas.findByLabelText(/^name/i), "test-app");
		const callbackInput = canvas.getByLabelText(/default callback/i);
		// oxlint-disable-next-line eslint/no-script-url -- Deliberately invalid input exercises callback URL rejection.
		await userEvent.type(callbackInput, "javascript:alert(1)");
		await userEvent.tab();
	},
};

export const DynamicallyRegisteredValues: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.type(
			await canvas.findByLabelText(/^name/i),
			"VS Code Coder Extension",
		);
		await userEvent.type(
			canvas.getByLabelText(/default callback/i),
			"vscode://coder.coder-remote/oauth/callback",
		);
		await userEvent.tab();
	},
};

export const MultipleRedirectURIs: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.type(
			await canvas.findByLabelText(/^name/i),
			"VS Code Coder Extension",
		);
		await userEvent.type(
			canvas.getByLabelText(/default callback/i),
			"vscode://coder.coder-remote/oauth/callback",
		);
		await userEvent.click(
			canvas.getByRole("button", { name: /add redirect uri/i }),
		);
		await userEvent.type(
			canvas.getByLabelText(/^redirect uri 2/i),
			"https://example.com/callback",
		);
	},
};

export const InvalidRowState: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.type(await canvas.findByLabelText(/^name/i), "test-app");
		await userEvent.click(
			canvas.getByRole("button", { name: /add redirect uri/i }),
		);
		// oxlint-disable-next-line eslint/no-script-url -- Deliberately invalid input exercises redirect URI rejection.
		const invalidRedirectURI = "javascript:alert(1)";
		await userEvent.type(
			canvas.getByLabelText(/^redirect uri 2/i),
			invalidRedirectURI,
		);
		await userEvent.tab();
	},
};

export const NoPermissions: Story = {
	parameters: {
		permissions: {
			...MockPermissions,
			createOAuth2App: false,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			await canvas.findByRole("button", { name: /create application/i }),
		).toBeDisabled();
	},
};
