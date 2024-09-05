import type { Meta, StoryObj } from "@storybook/react";
import { MockAuthMethodsAll, MockUser } from "testHelpers/entities";
import {
	withAuthProvider,
	withDashboardProvider,
	withGlobalSnackbar,
} from "testHelpers/storybook";
import UsersPage from "./UsersPage";
import { groupsQueryKey } from "api/queries/groups";
import { MockGroups } from "pages/UsersPage/storybookData/groups";
import { authMethodsQueryKey, usersKey } from "api/queries/users";
import { rolesQueryKey } from "api/queries/roles";
import { MockRoles } from "pages/UsersPage/storybookData/roles";
import { MockUsers } from "pages/UsersPage/storybookData/users";
import { deploymentConfigQueryKey } from "api/queries/deployment";
import { spyOn, userEvent, within } from "@storybook/test";
import { API } from "api/api";

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
	user: MockUser,
	permissions: {
		createUser: true,
		updateUsers: true,
		viewDeploymentValues: true,
	},
};

const meta: Meta<typeof UsersPage> = {
	title: "pages/UsersPage",
	component: UsersPage,
	parameters,
	decorators: [withGlobalSnackbar, withAuthProvider, withDashboardProvider],
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
		spyOn(API, "suspendUser").mockResolvedValue({
			...MockUsers[0],
			status: "suspended",
		});

		await user.click(within(userRow).getByLabelText("More options"));
		const suspendButton = await within(userRow).findByText("Suspend", {
			exact: false,
		});
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
		spyOn(API, "suspendUser").mockRejectedValue({});

		await user.click(within(userRow).getByLabelText("More options"));
		const suspendButton = await within(userRow).findByText("Suspend", {
			exact: false,
		});
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
		spyOn(API, "deleteUser").mockResolvedValue();

		await user.click(within(userRow).getByLabelText("More options"));
		const deleteButton = await within(userRow).findByText("Delete", {
			exact: false,
		});
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

		await user.click(within(userRow).getByLabelText("More options"));
		const deleteButton = await within(userRow).findByText("Delete", {
			exact: false,
		});
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
					users: MockUsers.map((u, i) => {
						return i === 0 ? { ...u, status: "suspended" } : u;
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
		spyOn(API, "activateUser").mockResolvedValue({
			...MockUsers[0],
			status: "active",
		});

		await user.click(within(userRow).getByLabelText("More options"));
		const activateButton = await within(userRow).findByText("Activate", {
			exact: false,
		});
		await user.click(activateButton);

		const dialog = await within(document.body).findByRole("dialog");
		await user.click(within(dialog).getByRole("button", { name: "Activate" }));
		await within(document.body).findByText("Successfully activate the user.");
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

		await user.click(within(userRow).getByLabelText("More options"));
		const activateButton = await within(userRow).findByText("Activate", {
			exact: false,
		});
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

		await user.click(within(userRow).getByLabelText("More options"));
		const resetPasswordButton = await within(userRow).findByText(
			"Reset password",
			{ exact: false },
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

		await user.click(within(userRow).getByLabelText("More options"));
		const resetPasswordButton = await within(userRow).findByText(
			"Reset password",
			{ exact: false },
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
					users: MockUsers.map((u, i) => {
						return i === 0
							? {
									...u,
									roles: [
										{
											name: "owner",
											display_name: "Owner",
										},
									],
								}
							: u;
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
		spyOn(API, "updateUserRoles").mockResolvedValue({
			...MockUsers[0],
			roles: [
				{ name: "owner", display_name: "Owner" },
				// We will update the user role to include auditor
				{ name: "auditor", display_name: "Auditor" },
			],
		});

		await user.click(within(userRow).getByLabelText("Edit user roles"));
		await user.click(
			within(userRow).getByLabelText("Auditor", { exact: false }),
		);
		await within(document.body).findByText(
			"Successfully updated the user roles.",
		);
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
		await user.click(
			within(userRow).getByLabelText("Auditor", { exact: false }),
		);
		await within(document.body).findByText("Error updating the user roles.");
	},
};
