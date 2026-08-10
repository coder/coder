import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import {
	oauth2ProviderAppsKey,
	oauth2ProviderSettingsKey,
} from "#/api/queries/oauth2";
import {
	MockOAuth2ProviderApps,
	MockOAuth2ProviderSettings,
	MockUserOwner,
} from "#/testHelpers/entities";
import { withAuthProvider } from "#/testHelpers/storybook";
import OAuth2AppsSettingsPage from "./OAuth2AppsSettingsPage";

/**
 * Presentation is covered by the view and the setting component. These stories
 * exist for the one thing only the container does: turn two RBAC permissions
 * into the props that decide whether an admin is offered a deployment-wide
 * security switch. Reading the wrong permission produces a page that looks
 * correct and fails at the API.
 */
const meta: Meta<typeof OAuth2AppsSettingsPage> = {
	title: "pages/DeploymentSettingsPage/OAuth2AppsSettingsPage",
	component: OAuth2AppsSettingsPage,
	decorators: [withAuthProvider],
	parameters: {
		user: MockUserOwner,
		queries: [
			{ key: oauth2ProviderAppsKey, data: MockOAuth2ProviderApps },
			{ key: oauth2ProviderSettingsKey, data: MockOAuth2ProviderSettings },
		],
	},
};

export default meta;
type Story = StoryObj<typeof OAuth2AppsSettingsPage>;

export const CanEditSettings: Story = {
	parameters: {
		permissions: {
			createOAuth2App: true,
			viewDeploymentConfig: true,
			editDeploymentConfig: true,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("tab", { name: "Settings" }));

		await expect(canvas.getByRole("button", { name: "Enable" })).toBeEnabled();
	},
};

/**
 * `editDeploymentConfig` is the update permission the endpoint enforces, and
 * the Auditor role holds read without it. Deriving the button from the read
 * permission instead hands that role a live control that 403s.
 */
export const ViewOnlyCannotEditSettings: Story = {
	parameters: {
		permissions: {
			createOAuth2App: true,
			viewDeploymentConfig: true,
			editDeploymentConfig: false,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("tab", { name: "Settings" }));

		await expect(canvas.getByRole("button", { name: "Enable" })).toBeDisabled();
		await expect(
			canvas.getByText(/permission to edit deployment configuration/),
		).toBeVisible();
	},
};

/**
 * Edit is granted here and view is not, which is not a combination RBAC
 * produces. It is the point: the tab has to follow the read permission, so
 * granting the other one must not reveal it.
 */
export const WithoutViewPermissionHidesSettings: Story = {
	parameters: {
		permissions: {
			createOAuth2App: true,
			viewDeploymentConfig: false,
			editDeploymentConfig: true,
		},
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
			canvas.queryByRole("button", { name: "Enable" }),
		).not.toBeInTheDocument();
	},
};
