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
import { SlimRole } from "api/typesGenerated";

const parameters = {
	queries: [
		// TODO: Investigate the reason behind the UI making two query calls:
		//       1. One with offset 0
		//       2. Another with offset 25
		{
			key: usersKey({ limit: 25, offset: 25, q: "" }),
			data: {
				users: MockUsers,
				count: 60,
			},
		},
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
		const firstUserRow = canvasElement.querySelector<HTMLElement>("tbody tr");
		if (!firstUserRow) {
			throw new Error("No user row found");
		}
		spyOn(API, "suspendUser").mockResolvedValue({
			...MockUsers[0],
			status: "suspended",
		});

		await suspendUser(user, firstUserRow);

		await within(document.body).findByText("Successfully suspended the user.");
	},
};

export const SuspendUserError: Story = {
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const firstUserRow = canvasElement.querySelector<HTMLElement>("tbody tr");
		if (!firstUserRow) {
			throw new Error("No user row found");
		}
		spyOn(API, "suspendUser").mockRejectedValue({});

		await suspendUser(user, firstUserRow);

		await within(document.body).findByText("Error suspending user.");
	},
};

export const DeleteUserSuccess: Story = {
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const firstUserRow = canvasElement.querySelector<HTMLElement>("tbody tr");
		if (!firstUserRow) {
			throw new Error("No user row found");
		}
		spyOn(API, "deleteUser").mockResolvedValue();

		await deleteUser(user, firstUserRow);

		await within(document.body).findByText("Successfully deleted the user.");
	},
};

export const DeleteUserError: Story = {
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const firstUserRow = canvasElement.querySelector<HTMLElement>("tbody tr");
		if (!firstUserRow) {
			throw new Error("No user row found");
		}
		spyOn(API, "deleteUser").mockRejectedValue({});

		await deleteUser(user, firstUserRow);

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
				key: usersKey({ limit: 25, offset: 25, q: "" }),
				data: {
					users: MockUsers.map((u, i) => {
						return i === 0 ? { ...u, status: "suspended" } : u;
					}),
					count: 60,
				},
			},
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
		const firstUserRow = canvasElement.querySelector<HTMLElement>("tbody tr");
		if (!firstUserRow) {
			throw new Error("No user row found");
		}
		spyOn(API, "activateUser").mockResolvedValue({
			...MockUsers[0],
			status: "active",
		});

		await activateUser(user, firstUserRow);

		await within(document.body).findByText("Successfully activate the user.");
	},
};

export const ActivateUserError: Story = {
	parameters: {
		queries: [
			...parameters.queries,
			// To activate a user, the user must be suspended first. Since we use the
			// first user in the test, we need to ensure it is suspended.
			{
				key: usersKey({ limit: 25, offset: 25, q: "" }),
				data: {
					users: MockUsers.map((u, i) => {
						return i === 0 ? { ...u, status: "suspended" } : u;
					}),
					count: 60,
				},
			},
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
		const firstUserRow = canvasElement.querySelector<HTMLElement>("tbody tr");
		if (!firstUserRow) {
			throw new Error("No user row found");
		}
		spyOn(API, "activateUser").mockRejectedValue({});

		await activateUser(user, firstUserRow);

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
				key: usersKey({ limit: 25, offset: 25, q: "" }),
				data: {
					users: MockUsers.map((u, i) => {
						return i === 0 ? { ...u, login_type: "password" } : u;
					}),
					count: 60,
				},
			},
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
		const firstUserRow = canvasElement.querySelector<HTMLElement>("tbody tr");
		if (!firstUserRow) {
			throw new Error("No user row found");
		}
		spyOn(API, "updateUserPassword").mockResolvedValue();

		await resetUserPassword(user, firstUserRow);

		await within(document.body).findByText(
			"Successfully updated the user password.",
		);
	},
};

export const ResetUserPasswordError: Story = {
	parameters: {
		queries: [
			...parameters.queries,
			// Ensure the first user's login type is set to 'password' to reset their
			// password during the test.
			{
				key: usersKey({ limit: 25, offset: 25, q: "" }),
				data: {
					users: MockUsers.map((u, i) => {
						return i === 0 ? { ...u, login_type: "password" } : u;
					}),
					count: 60,
				},
			},
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
		const firstUserRow = canvasElement.querySelector<HTMLElement>("tbody tr");
		if (!firstUserRow) {
			throw new Error("No user row found");
		}
		spyOn(API, "updateUserPassword").mockRejectedValue({});

		await resetUserPassword(user, firstUserRow);

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
				key: usersKey({ limit: 25, offset: 25, q: "" }),
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
		const firstUserRow = canvasElement.querySelector<HTMLElement>("tbody tr");
		if (!firstUserRow) {
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

		await user.click(within(firstUserRow).getByLabelText("Edit user roles"));
		await user.click(
			within(firstUserRow).getByLabelText("Auditor", { exact: false }),
		);
		await within(document.body).findByText(
			"Successfully updated the user roles.",
		);
	},
};

export const UpdateUserRoleError: Story = {
	parameters: {
		queries: [
			...parameters.queries,
			//	Ensure the first user has the 'owner' role to test the edit functionality.
			{
				key: usersKey({ limit: 25, offset: 25, q: "" }),
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
		const firstUserRow = canvasElement.querySelector<HTMLElement>("tbody tr");
		if (!firstUserRow) {
			throw new Error("No user row found");
		}
		spyOn(API, "updateUserRoles").mockRejectedValue({});

		await user.click(within(firstUserRow).getByLabelText("Edit user roles"));
		await user.click(
			within(firstUserRow).getByLabelText("Auditor", { exact: false }),
		);
		await within(document.body).findByText("Error updating the user roles.");
	},
};

async function suspendUser(
	user: ReturnType<typeof userEvent.setup>,
	userRow: HTMLElement,
) {
	// Open "More options" menu
	const moreOptionsButton = within(userRow).getByLabelText("More options");
	await user.click(moreOptionsButton);

	// Click on "Suspend..."
	const suspendButton = await within(userRow).findByText("Suspend", {
		exact: false,
	});
	await user.click(suspendButton);

	// Confirm the suspension by clicking on "Suspend" button in the dialog
	const dialog = await within(document.body).findByRole("dialog");
	await user.click(within(dialog).getByRole("button", { name: "Suspend" }));
}

async function deleteUser(
	user: ReturnType<typeof userEvent.setup>,
	userRow: HTMLElement,
) {
	// Open "More options" menu
	const moreOptionsButton = within(userRow).getByLabelText("More options");
	await user.click(moreOptionsButton);

	// Click on "Delete..."
	const deleteButton = await within(userRow).findByText("Delete", {
		exact: false,
	});
	await user.click(deleteButton);

	// Wait for the dialog
	const dialog = await within(document.body).findByRole("dialog");

	// Confirm the deletion by typing the user name and clicking on "Delete"
	// button in the dialog
	const input = within(dialog).getByLabelText("Name of the user to delete");
	await user.type(input, MockUsers[0].username);
	await user.click(within(dialog).getByRole("button", { name: "Delete" }));
}

async function activateUser(
	user: ReturnType<typeof userEvent.setup>,
	userRow: HTMLElement,
) {
	// Open "More options" menu
	const moreOptionsButton = within(userRow).getByLabelText("More options");
	await user.click(moreOptionsButton);

	// Click on "Activate..."
	const activateButton = await within(userRow).findByText("Activate", {
		exact: false,
	});
	await user.click(activateButton);

	// Wait for the dialog
	const dialog = await within(document.body).findByRole("dialog");

	// Confirm the activation by clicking on "Activate" button in the dialog
	await user.click(within(dialog).getByRole("button", { name: "Activate" }));
}

async function resetUserPassword(
	user: ReturnType<typeof userEvent.setup>,
	userRow: HTMLElement,
) {
	// Open "More options" menu
	const moreOptionsButton = within(userRow).getByLabelText("More options");
	await user.click(moreOptionsButton);

	// Click on "Reset password..."
	const resetPasswordButton = await within(userRow).findByText(
		"Reset password",
		{
			exact: false,
		},
	);
	await user.click(resetPasswordButton);

	// Wait for the dialog
	const dialog = await within(document.body).findByRole("dialog");

	// Confirm the activation by clicking on "Reset password" button in the dialog
	await user.click(
		within(dialog).getByRole("button", { name: "Reset password" }),
	);
}
