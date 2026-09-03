import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, spyOn, userEvent, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { API } from "#/api/api";
import {
	oauth2ProviderAppKey,
	oauth2ProviderAppSecretsKey,
} from "#/api/queries/oauth2";
import {
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
					{ field: "callback_url", detail: "url error" },
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

export const NoSecretPermissions: Story = {
	parameters: {
		permissions: {
			...MockPermissions,
			viewOAuth2AppSecrets: false,
			deleteOAuth2App: false,
		},
		queries: [{ key: oauth2ProviderAppKey(appId), data: mockApp }],
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
