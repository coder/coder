import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, spyOn, userEvent, waitFor, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { API } from "#/api/api";
import {
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
					{ field: "callback_url", detail: "url error" },
					{ field: "icon", detail: "icon error" },
				],
			}),
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.type(await canvas.findByLabelText(/^name/i), "test-app");
		await userEvent.type(
			canvas.getByLabelText(/callback url/i),
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

export const InvalidName: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const nameInput = await canvas.findByLabelText(/^name/i);
		await userEvent.type(nameInput, "Foo@Application");
		await userEvent.tab();
		await expect(
			await canvas.findByText(
				/special characters \(e\.g\.: !, @, #\) are not supported/i,
			),
		).toBeVisible();
		await expect(
			canvas.getByRole("button", { name: /create application/i }),
		).toBeDisabled();
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
