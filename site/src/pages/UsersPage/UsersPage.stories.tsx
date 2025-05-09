import type { Meta, StoryObj } from "@storybook/react";
import { screen, spyOn, userEvent, within } from "@storybook/test";
import { API } from "api/api";
import { deploymentConfigQueryKey } from "api/queries/deployment";
import { groupsQueryKey } from "api/queries/groups";
import { rolesQueryKey } from "api/queries/roles";
import { authMethodsQueryKey, usersKey } from "api/queries/users";
import type { User } from "api/typesGenerated";
import { MockGroups } from "pages/UsersPage/storybookData/groups";
import { MockRoles } from "pages/UsersPage/storybookData/roles";
import { MockUsers } from "pages/UsersPage/storybookData/users";
import { MockAuthMethodsAll, MockUserOwner } from "testHelpers/entities";
import {
	withAuthProvider,
	withDashboardProvider,
	withGlobalSnackbar,
} from "testHelpers/storybook";
import UsersPage from "./UsersPage";

const parameters = {
	queries: [
		// This query loads users for the filter menu, not for the table
		{
			key: usersKey({ limit: 25, offset: 25, q: "" }),
			data: {
				users: [],
				count: 60,
			},
		},
		// Users for the table
		{
			key: usersKey({ limit: 25, offset: 0, q: "" }),
			data: {
				users: MockUsers,
				count: 60,
			},
		},
		{
			key: groupsQueryKey,
			data: MockGroups,
		},
		{
			key: authMethodsQueryKey,
			data: MockAuthMethodsAll,
		},
		{
			key: rolesQueryKey,
			data: MockRoles,
		},
		{
			key: deploymentConfigQueryKey,
			data: {
				config: {
					oidc: {
						user_role_field: "role",
					},
				},
				options: [],
			},
		},
	],
	user: MockUserOwner,
	permissions: {
		createUser: true,
		updateUsers: true,
		viewDeploymentConfig: true,
	},
};

const meta: Meta<typeof UsersPage> = {
	title: "pages/UsersPage",
	component: UsersPage,
	parameters,
	decorators: [withGlobalSnackbar, withAuthProvider, withDashboardProvider],
	args: {
		defaultNewPassword: "edWbqYiaVpEiEWwI",
	},
};

export default meta;
type Story = StoryObj<typeof UsersPage>;

export const Loaded: Story = {};

export const SuspendUserSuccess: Story = {
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const userRow = canvasElement.querySelector<HTMLElement>("tbody tr");
		if (!userRow) {
			throw new Error("No user row found");
		}

		// Return the updated user in the suspended response and ensure the users
		// query will return updated data.
		const updatedUser: User = { ...MockUsers[0], status: "suspended" };
		spyOn(API, "suspendUser").mockResolvedValue(updatedUser);
		spyOn(API, "getUsers").mockResolvedValue({
			users: replaceUser(MockUsers, 0, updatedUser),
			count: 60,
		});

		await user.click(within(userRow).getByLabelText("Open menu"));
		const suspendButton = await within(document.body).findByText("Suspend…");
		await user.click(suspendButton);

		const dialog = await within(document.body).findByRole("dialog");
		await user.click(within(dialog).getByRole("button", { name: "Suspend" }));
		await within(document.body).findByText("Successfully suspended the user.");
	},
};

export const SuspendUserError: Story = {
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const userRow = canvasElement.querySelector<HTMLElement>("tbody tr");
		if (!userRow) {
			throw new Error("No user row found");
		}
		spyOn(API, "suspendUser").mockRejectedValue(undefined);

		await user.click(within(userRow).getByLabelText("Open menu"));
		const suspendButton = await within(document.body).findByText("Suspend…");
		await user.click(suspendButton);

		const dialog = await within(document.body).findByRole("dialog");
		await user.click(within(dialog).getByRole("button", { name: "Suspend" }));
		await within(document.body).findByText("Error suspending user.");
	},
};

export const DeleteUserSuccess: Story = {
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const userRow = canvasElement.querySelector<HTMLElement>("tbody tr");
		if (!userRow) {
			throw new Error("No user row found");
		}

		// The delete user operation does not return a value. However, we need to
		// ensure that the updated list of users, excluding the deleted one, is
		// returned when the users query is refetched.
		spyOn(API, "deleteUser").mockResolvedValue();
		spyOn(API, "getUsers").mockResolvedValue({
			users: MockUsers.slice(1),
			count: 59,
		});

		await user.click(within(userRow).getByLabelText("Open menu"));
		const deleteButton = await within(document.body).findByText("Delete…");
		await user.click(deleteButton);

		const dialog = await within(document.body).findByRole("dialog");
		const input = within(dialog).getByLabelText("Name of the user to delete");
		await user.type(input, MockUsers[0].username);
		await user.click(within(dialog).getByRole("button", { name: "Delete" }));
		await within(document.body).findByText("Successfully deleted the user.");
	},
};

export const DeleteUserError: Story = {
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const userRow = canvasElement.querySelector<HTMLElement>("tbody tr");
		if (!userRow) {
			throw new Error("No user row found");
		}
		spyOn(API, "deleteUser").mockRejectedValue({});

		await user.click(within(userRow).getByLabelText("Open menu"));
		const deleteButton = await within(document.body).findByText("Delete…");
		await user.click(deleteButton);

		const dialog = await within(document.body).findByRole("dialog");
		const input = within(dialog).getByLabelText("Name of the user to delete");
		await user.type(input, MockUsers[0].username);
		await user.click(within(dialog).getByRole("button", { name: "Delete" }));
		await within(document.body).findByText("Error deleting user.");
	},
};

export const ActivateUserSuccess: Story = {
	parameters: {
		queries: [
			...parameters.queries,
			// To activate a user, the user must be suspended first. Since we use the
			// first user in the test, we need to ensure it is suspended.
			{
				key: usersKey({ limit: 25, offset: 0, q: "" }),
				data: {
					users: replaceUser(MockUsers, 0, {
						...MockUsers[0],
						status: "suspended",
					}),
					count: 60,
				},
			},
		],
	},
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const userRow = canvasElement.querySelector<HTMLElement>("tbody tr");
		if (!userRow) {
			throw new Error("No user row found");
		}

		// Return the updated user in the activate response and ensure the users
		// query will return updated data.
		const updatedUser: User = { ...MockUsers[0], status: "active" };
		spyOn(API, "activateUser").mockResolvedValue(updatedUser);
		spyOn(API, "getUsers").mockResolvedValue({
			users: replaceUser(MockUsers, 0, updatedUser),
			count: 60,
		});

		await user.click(within(userRow).getByLabelText("Open menu"));
		const activateButton = await within(document.body).findByText("Activate…");
		await user.click(activateButton);

		const dialog = await within(document.body).findByRole("dialog");
		await user.click(within(dialog).getByRole("button", { name: "Activate" }));
		await within(document.body).findByText("Successfully activated the user.");
	},
};

export const ActivateUserError: Story = {
	parameters: ActivateUserSuccess.parameters,
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const userRow = canvasElement.querySelector<HTMLElement>("tbody tr");
		if (!userRow) {
			throw new Error("No user row found");
		}
		spyOn(API, "activateUser").mockRejectedValue({});

		await user.click(within(userRow).getByLabelText("Open menu"));
		const activateButton = await within(document.body).findByText("Activate…");
		await user.click(activateButton);

		const dialog = await within(document.body).findByRole("dialog");
		await user.click(within(dialog).getByRole("button", { name: "Activate" }));
		await within(document.body).findByText("Error activating user.");
	},
};

export const ResetUserPasswordSuccess: Story = {
	parameters: {
		queries: [
			...parameters.queries,
			// Ensure the first user's login type is set to 'password' to reset their
			// password during the test.
			{
				key: usersKey({ limit: 25, offset: 0, q: "" }),
				data: {
					users: MockUsers.map((u, i) => {
						return i === 0 ? { ...u, login_type: "password" } : u;
					}),
					count: 60,
				},
			},
		],
	},
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const userRow = canvasElement.querySelector<HTMLElement>("tbody tr");
		if (!userRow) {
			throw new Error("No user row found");
		}
		spyOn(API, "updateUserPassword").mockResolvedValue();

		await user.click(within(userRow).getByLabelText("Open menu"));
		const resetPasswordButton = await within(document.body).findByText(
			"Reset password…",
		);
		await user.click(resetPasswordButton);

		const dialog = await within(document.body).findByRole("dialog");
		await user.click(
			within(dialog).getByRole("button", { name: "Reset password" }),
		);
		await within(document.body).findByText(
			"Successfully updated the user password.",
		);
	},
};

export const ResetUserPasswordError: Story = {
	parameters: ResetUserPasswordSuccess.parameters,
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const userRow = canvasElement.querySelector<HTMLElement>("tbody tr");
		if (!userRow) {
			throw new Error("No user row found");
		}
		spyOn(API, "updateUserPassword").mockRejectedValue({});

		await user.click(within(userRow).getByLabelText("Open menu"));
		const resetPasswordButton = await within(document.body).findByText(
			"Reset password…",
		);
		await user.click(resetPasswordButton);

		const dialog = await within(document.body).findByRole("dialog");
		await user.click(
			within(dialog).getByRole("button", { name: "Reset password" }),
		);
		await within(document.body).findByText(
			"Error on resetting the user password.",
		);
	},
};

export const UpdateUserRoleSuccess: Story = {
	parameters: {
		queries: [
			...parameters.queries,
			//	Ensure the first user has the 'owner' role to test the edit functionality.
			{
				key: usersKey({ limit: 25, offset: 0, q: "" }),
				data: {
					users: replaceUser(MockUsers, 0, {
						...MockUsers[0],
						roles: [
							{ name: "owner", display_name: "Owner" },
							// We will update the user role to include auditor
							{ name: "auditor", display_name: "Auditor" },
						],
					}),
					count: 60,
				},
			},
		],
	},
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const userRow = canvasElement.querySelector<HTMLElement>("tbody tr");
		if (!userRow) {
			throw new Error("No user row found");
		}

		// Return the updated user in the update roles response and ensure the users
		// query will return updated data.
		const updatedUser: User = {
			...MockUsers[0],
			roles: [
				{ name: "owner", display_name: "Owner" },
				// We will update the user role to include auditor
				{ name: "auditor", display_name: "Auditor" },
			],
		};
		spyOn(API, "updateUserRoles").mockResolvedValue(updatedUser);
		spyOn(API, "getUsers").mockResolvedValue({
			users: replaceUser(MockUsers, 0, updatedUser),
			count: 60,
		});

		await user.click(within(userRow).getByLabelText("Edit user roles"));
		await user.click(screen.getByLabelText("Auditor", { exact: false }));
		await screen.findByText("Successfully updated the user roles.");
	},
};

export const UpdateUserRoleError: Story = {
	parameters: UpdateUserRoleSuccess.parameters,
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const userRow = canvasElement.querySelector<HTMLElement>("tbody tr");
		if (!userRow) {
			throw new Error("No user row found");
		}
		spyOn(API, "updateUserRoles").mockRejectedValue({});

		await user.click(within(userRow).getByLabelText("Edit user roles"));
		await user.click(screen.getByLabelText("Auditor", { exact: false }));
		await screen.findByText("Error on updating the user roles.");
	},
};

function replaceUser(users: User[], index: number, user: User) {
	return users.map((u, i) => (i === index ? user : u));
}
