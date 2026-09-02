import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, spyOn, userEvent, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { API } from "#/api/api";
import { deploymentConfigQueryKey } from "#/api/queries/deployment";
import { userKey } from "#/api/queries/users";
import {
	MockUserMember,
	MockUserOwner,
	mockApiError,
} from "#/testHelpers/entities";
import {
	withAuthProvider,
	withDashboardProvider,
	withToaster,
} from "#/testHelpers/storybook";
import EditUserPage from "./EditUserPage";

const UsersListRoute = () => <div>Users list</div>;

const meta: Meta<typeof EditUserPage> = {
	title: "pages/EditUserPage/EditUserPage",
	component: EditUserPage,
	decorators: [withToaster, withAuthProvider, withDashboardProvider],
	parameters: {
		user: MockUserOwner,
		permissions: {
			updateUsers: true,
			viewDeploymentConfig: true,
		},
		features: ["audit_log"],
		queries: [
			{ key: userKey(MockUserMember.username), data: MockUserMember },
			{
				key: deploymentConfigQueryKey,
				data: {
					config: { oidc: { user_role_field: "" } },
					options: [],
				},
			},
		],
		reactRouter: reactRouterParameters({
			location: { path: `/deployment/users/${MockUserMember.username}` },
			routing: [
				{ path: "/deployment/users", Component: UsersListRoute },
				{ path: "/deployment/users/:user", useStoryElement: true },
			],
		}),
	},
};

export default meta;
type Story = StoryObj<typeof EditUserPage>;

export const Ready: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			await canvas.findByRole("heading", {
				name: `Edit ${MockUserMember.name}`,
			}),
		).toBeVisible();
		await userEvent.click(canvas.getByRole("button", { name: /open menu/i }));
		const menu = within(document.body);
		await menu.findByRole("menuitem", { name: "View workspaces" });
		await menu.findByRole("menuitem", { name: "View activity" });
		await expect(
			menu.queryByRole("menuitem", { name: "Edit" }),
		).not.toBeInTheDocument();
		await menu.findByRole("menuitem", { name: "Delete…" });
	},
};

export const WithoutUpdatePermission: Story = {
	parameters: {
		permissions: {
			updateUsers: false,
			viewDeploymentConfig: true,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			await canvas.findByRole("heading", {
				name: `Edit ${MockUserMember.name}`,
			}),
		).toBeVisible();
		await expect(
			canvas.queryByRole("button", { name: /open menu/i }),
		).not.toBeInTheDocument();
		await expect(canvas.getByLabelText(/username/i)).toBeVisible();
	},
};

export const DeletesAndReturnsToUsers: Story = {
	beforeEach: () => {
		spyOn(API, "deleteUser").mockResolvedValue();
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await canvas.findByRole("heading", {
			name: `Edit ${MockUserMember.name}`,
		});
		await userEvent.click(canvas.getByRole("button", { name: /open menu/i }));
		await userEvent.click(
			await within(document.body).findByRole("menuitem", { name: "Delete…" }),
		);
		const dialog = await within(document.body).findByRole("dialog");
		await userEvent.type(
			within(dialog).getByLabelText("Name of the user to delete"),
			MockUserMember.username,
		);
		await userEvent.click(
			within(dialog).getByRole("button", { name: "Delete" }),
		);
		await expect(await canvas.findByText("Users list")).toBeVisible();
		await expect(
			canvas.queryByRole("heading", { name: `Edit ${MockUserMember.name}` }),
		).not.toBeInTheDocument();
		expect(API.deleteUser).toHaveBeenCalledWith(MockUserMember.id);
	},
};

export const Loading: Story = {
	parameters: {
		queries: [
			{
				key: deploymentConfigQueryKey,
				data: {
					config: { oidc: { user_role_field: "" } },
					options: [],
				},
			},
		],
	},
	beforeEach: () => {
		spyOn(API, "getUser").mockReturnValue(new Promise(() => {}));
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("status", { name: /loading/i }),
		).toBeVisible();
		await expect(
			canvas.queryByRole("heading", { name: /edit/i }),
		).not.toBeInTheDocument();
	},
};

export const UserError: Story = {
	parameters: {
		queries: [
			{
				key: deploymentConfigQueryKey,
				data: {
					config: { oidc: { user_role_field: "" } },
					options: [],
				},
			},
		],
	},
	beforeEach: () => {
		spyOn(API, "getUser").mockRejectedValue(
			mockApiError({ message: "User not found." }),
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(await canvas.findByText("User not found.")).toBeVisible();
		await expect(
			canvas.queryByRole("heading", { name: /edit/i }),
		).not.toBeInTheDocument();
	},
};
