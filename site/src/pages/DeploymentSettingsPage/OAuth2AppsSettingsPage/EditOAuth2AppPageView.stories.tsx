import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, spyOn, userEvent, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { API } from "#/api/api";
import {
	externalScopesKey,
	oauth2ProviderAppKey,
	oauth2ProviderAppSecretsKey,
} from "#/api/queries/oauth2";
import {
	MockExternalAPIKeyScopes,
	MockOAuth2ProviderAppPublic,
	MockOAuth2ProviderAppSecrets,
	MockOAuth2ProviderApps,
	MockPermissions,
	MockUserOwner,
	mockApiError,
} from "#/testHelpers/entities";
import { withAuthProvider, withToaster } from "#/testHelpers/storybook";
import { EditOAuth2AppPageView } from "./EditOAuth2AppPageView";

const mockApp = MockOAuth2ProviderApps[0];
const appId = mockApp.id;

const routingFor = (path: string) =>
	reactRouterParameters({
		location: { path },
		routing: [
			{ path: "/deployment/oauth2-provider/apps", useStoryElement: true },
			{
				path: "/deployment/oauth2-provider/apps/:appId",
				useStoryElement: true,
			},
		],
	});

const meta = {
	title: "pages/DeploymentSettingsPage/EditOAuth2AppPageView",
	component: EditOAuth2AppPageView,
	parameters: {
		user: MockUserOwner,
		permissions: MockPermissions,
		reactRouter: routingFor(`/deployment/oauth2-provider/apps/${appId}`),
	},
	decorators: [withToaster, withAuthProvider],
} satisfies Meta<typeof EditOAuth2AppPageView>;

export default meta;
type Story = StoryObj<typeof EditOAuth2AppPageView>;

export const Default: Story = {
	parameters: {
		queries: [
			{ key: externalScopesKey, data: MockExternalAPIKeyScopes },
			{ key: oauth2ProviderAppKey(appId), data: mockApp },
			{
				key: oauth2ProviderAppSecretsKey(appId),
				data: MockOAuth2ProviderAppSecrets,
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(await canvas.findByText(mockApp.name)).toBeVisible();
		await expect(
			canvas.getByRole("button", { name: /update application/i }),
		).toBeVisible();
		await expect(
			canvas.getByRole("table", { name: "OAuth2 client secrets" }),
		).toBeVisible();
	},
};

export const EmptySecrets: Story = {
	parameters: {
		queries: [
			{ key: externalScopesKey, data: MockExternalAPIKeyScopes },
			{ key: oauth2ProviderAppKey(appId), data: mockApp },
			{
				key: oauth2ProviderAppSecretsKey(appId),
				data: [],
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(await canvas.findByText(mockApp.name)).toBeVisible();
		await expect(
			canvas.getByRole("table", { name: "OAuth2 client secrets" }),
		).toBeVisible();
		await expect(
			canvas.getByText("No client secrets have been generated."),
		).toBeVisible();
	},
};

export const Loading: Story = {
	parameters: {
		queries: [],
	},
	beforeEach: () => {
		spyOn(API, "getOAuth2ProviderApp").mockReturnValue(new Promise(() => {}));
	},
};

export const WithValidationError: Story = {
	parameters: {
		queries: [
			{ key: externalScopesKey, data: MockExternalAPIKeyScopes },
			{ key: oauth2ProviderAppKey(appId), data: mockApp },
			{
				key: oauth2ProviderAppSecretsKey(appId),
				data: MockOAuth2ProviderAppSecrets,
			},
		],
	},
	beforeEach: () => {
		spyOn(API, "putOAuth2ProviderApp").mockRejectedValue(
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
		await userEvent.type(await canvas.findByLabelText(/^name/i), "-updated");
		const submit = await canvas.findByRole("button", {
			name: /update application/i,
		});
		await userEvent.click(submit);
		await expect(await canvas.findByText("name error")).toBeVisible();
		await expect(canvas.getByText("url error")).toBeVisible();
		await expect(canvas.getByText("icon error")).toBeVisible();
	},
};

export const DeleteDialogOpen: Story = {
	parameters: {
		queries: [
			{ key: externalScopesKey, data: MockExternalAPIKeyScopes },
			{ key: oauth2ProviderAppKey(appId), data: mockApp },
			{
				key: oauth2ProviderAppSecretsKey(appId),
				data: MockOAuth2ProviderAppSecrets,
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const deleteButton = await canvas.findByRole("button", {
			name: /^delete$/i,
		});
		await userEvent.click(deleteButton);
		await expect(await screen.findByRole("dialog")).toBeInTheDocument();
		await expect(await screen.findByText(/irreversible/i)).toBeInTheDocument();
	},
};

export const DynamicallyRegisteredValues: Story = {
	parameters: {
		queries: [
			{ key: externalScopesKey, data: MockExternalAPIKeyScopes },
			{
				key: oauth2ProviderAppKey(appId),
				data: {
					...mockApp,
					name: "VS Code Coder Extension",
					callback_url: "vscode://coder.coder-remote/oauth/callback",
					redirect_uris: ["vscode://coder.coder-remote/oauth/callback"],
					dynamically_registered: true,
				},
			},
			{
				key: oauth2ProviderAppSecretsKey(appId),
				data: MockOAuth2ProviderAppSecrets,
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const nameField = await canvas.findByLabelText(/^name/i);
		await userEvent.clear(nameField);
		await userEvent.type(nameField, "Cursor (MCP)");
	},
};

export const PublicClient: Story = {
	parameters: {
		queries: [
			{ key: externalScopesKey, data: MockExternalAPIKeyScopes },
			{
				key: oauth2ProviderAppKey(MockOAuth2ProviderAppPublic.id),
				data: MockOAuth2ProviderAppPublic,
			},
		],
		reactRouter: routingFor(
			`/deployment/oauth2-provider/apps/${MockOAuth2ProviderAppPublic.id}`,
		),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			await canvas.findByText(MockOAuth2ProviderAppPublic.name),
		).toBeVisible();
		await expect(
			canvas.queryByRole("table", { name: "OAuth2 client secrets" }),
		).not.toBeInTheDocument();
		await expect(
			canvas.queryByRole("button", { name: /generate secret/i }),
		).not.toBeInTheDocument();
		await expect(await canvas.findByText(/public client/i)).toBeVisible();
	},
};

export const MultipleRedirectURIs: Story = {
	parameters: {
		queries: [
			{ key: externalScopesKey, data: MockExternalAPIKeyScopes },
			{
				key: oauth2ProviderAppKey(appId),
				data: {
					...mockApp,
					redirect_uris: [mockApp.callback_url, "https://example.com/callback"],
				},
			},
			{
				key: oauth2ProviderAppSecretsKey(appId),
				data: MockOAuth2ProviderAppSecrets,
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// Waits for the query to resolve so Pixel captures the loaded state.
		await canvas.findByLabelText(/^redirect uri 2/i);
	},
};

export const InvalidRowState: Story = {
	parameters: {
		queries: [
			{ key: externalScopesKey, data: MockExternalAPIKeyScopes },
			{ key: oauth2ProviderAppKey(appId), data: mockApp },
			{
				key: oauth2ProviderAppSecretsKey(appId),
				data: MockOAuth2ProviderAppSecrets,
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			await canvas.findByRole("button", { name: /add redirect uri/i }),
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

export const NoSecretPermissions: Story = {
	parameters: {
		permissions: {
			...MockPermissions,
			viewOAuth2AppSecrets: false,
			deleteOAuth2App: false,
		},
		queries: [
			{ key: externalScopesKey, data: MockExternalAPIKeyScopes },
			{ key: oauth2ProviderAppKey(appId), data: mockApp },
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(await canvas.findByText(mockApp.name)).toBeVisible();
		await expect(
			canvas.queryByRole("table", { name: "OAuth2 client secrets" }),
		).not.toBeInTheDocument();
		await expect(
			canvas.queryByRole("button", { name: /^delete$/i }),
		).not.toBeInTheDocument();
	},
};
