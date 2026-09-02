import type { Meta, StoryObj } from "@storybook/react-vite";
import { spyOn, userEvent, within } from "storybook/test";
import { API } from "#/api/api";
import { deploymentConfigQueryKey } from "#/api/queries/deployment";
import { groupsQueryKey } from "#/api/queries/groups";
import { rolesQueryKey } from "#/api/queries/roles";
import { authMethodsQueryKey, usersKey } from "#/api/queries/users";
import type { User } from "#/api/typesGenerated";
import { MockGroups } from "#/pages/UsersPage/storybookData/groups";
import { MockRoles } from "#/pages/UsersPage/storybookData/roles";
import { MockAuthMethodsAll, MockUserOwner } from "#/testHelpers/entities";
import {
	withAuthProvider,
	withDashboardProvider,
	withToaster,
} from "#/testHelpers/storybook";
import { MockUsers } from "#/testHelpers/users";
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
	decorators: [withToaster, withAuthProvider, withDashboardProvider],
};

export default meta;
type Story = StoryObj<typeof UsersPage>;

export const Loaded: Story = {};

export const SuspendUserSuccess: Story = {
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		const body = within(document.body);

		const updatedUser: User = { ...MockUsers[0], status: "suspended" };
		spyOn(API, "suspendUser").mockResolvedValue(updatedUser);
		spyOn(API, "getUsers").mockResolvedValue({
			users: replaceUser(MockUsers, 0, updatedUser),
			count: 60,
		});

		await openUserMenu(canvas, user);
		await user.click(await body.findByRole("menuitem", { name: "Suspend…" }));

		const dialog = await body.findByRole("dialog");
		await user.click(within(dialog).getByRole("button", { name: "Suspend" }));
		await body.findByText(/suspended successfully/);
	},
};

export const SuspendUserError: Story = {
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		const body = within(document.body);
		spyOn(API, "suspendUser").mockRejectedValue(undefined);

		await openUserMenu(canvas, user);
		await user.click(await body.findByRole("menuitem", { name: "Suspend…" }));

		const dialog = await body.findByRole("dialog");
		await user.click(within(dialog).getByRole("button", { name: "Suspend" }));
		await body.findByText(/Error suspending user/);
	},
};

export const DeleteUserSuccess: Story = {
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		const body = within(document.body);

		// The delete user operation does not return a value. However, we need to
		// ensure that the updated list of users, excluding the deleted one, is
		// returned when the users query is refetched.
		spyOn(API, "deleteUser").mockResolvedValue();
		spyOn(API, "getUsers").mockResolvedValue({
			users: MockUsers.slice(1),
			count: 59,
		});

		await openUserMenu(canvas, user);
		await user.click(await body.findByRole("menuitem", { name: "Delete…" }));

		const dialog = await body.findByRole("dialog");
		await user.type(
			within(dialog).getByLabelText("Name of the user to delete"),
			MockUsers[0].username,
		);
		await user.click(within(dialog).getByRole("button", { name: "Delete" }));
		await body.findByText(/deleted successfully/);
	},
};

export const DeleteUserError: Story = {
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		const body = within(document.body);
		spyOn(API, "deleteUser").mockRejectedValue({});

		await openUserMenu(canvas, user);
		await user.click(await body.findByRole("menuitem", { name: "Delete…" }));

		const dialog = await body.findByRole("dialog");
		await user.type(
			within(dialog).getByLabelText("Name of the user to delete"),
			MockUsers[0].username,
		);
		await user.click(within(dialog).getByRole("button", { name: "Delete" }));
		await body.findByText(/Error deleting user/);
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
		const canvas = within(canvasElement);
		const body = within(document.body);

		const updatedUser: User = { ...MockUsers[0], status: "active" };
		spyOn(API, "activateUser").mockResolvedValue(updatedUser);
		spyOn(API, "getUsers").mockResolvedValue({
			users: replaceUser(MockUsers, 0, updatedUser),
			count: 60,
		});

		await openUserMenu(canvas, user);
		await user.click(await body.findByRole("menuitem", { name: "Activate…" }));

		const dialog = await body.findByRole("dialog");
		await user.click(within(dialog).getByRole("button", { name: "Activate" }));
		await body.findByText(/activated successfully/);
	},
};

export const ActivateUserError: Story = {
	parameters: ActivateUserSuccess.parameters,
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		const body = within(document.body);
		spyOn(API, "activateUser").mockRejectedValue({});

		await openUserMenu(canvas, user);
		await user.click(await body.findByRole("menuitem", { name: "Activate…" }));

		const dialog = await body.findByRole("dialog");
		await user.click(within(dialog).getByRole("button", { name: "Activate" }));
		await body.findByText(/Error activating user/);
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
		const canvas = within(canvasElement);
		const body = within(document.body);
		spyOn(API, "updateUserPassword").mockResolvedValue();

		await openUserMenu(canvas, user);
		await user.click(
			await body.findByRole("menuitem", { name: "Reset password…" }),
		);

		const dialog = await body.findByRole("dialog");
		await user.click(
			within(dialog).getByRole("button", { name: "Reset password" }),
		);
		await body.findByText(/password .* updated successfully/i);
	},
};

export const ResetUserPasswordError: Story = {
	parameters: ResetUserPasswordSuccess.parameters,
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		const body = within(document.body);
		spyOn(API, "updateUserPassword").mockRejectedValue({});

		await openUserMenu(canvas, user);
		await user.click(
			await body.findByRole("menuitem", { name: "Reset password…" }),
		);

		const dialog = await body.findByRole("dialog");
		await user.click(
			within(dialog).getByRole("button", { name: "Reset password" }),
		);
		await body.findByText(/Error resetting password/i);
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
		const canvas = within(canvasElement);
		const body = within(document.body);

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

		await openUserMenu(canvas, user);
		await user.click(await body.findByRole("menuitem", { name: "Edit roles" }));
		const dialog = await body.findByRole("dialog");
		await user.click(
			within(dialog).getByLabelText("Auditor", { exact: false }),
		);
		await user.click(within(dialog).getByRole("button", { name: "Confirm" }));
		await body.findByText(/roles updated successfully/);
	},
};

export const UpdateUserRoleError: Story = {
	parameters: UpdateUserRoleSuccess.parameters,
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		const body = within(document.body);
		spyOn(API, "updateUserRoles").mockRejectedValue({});

		await openUserMenu(canvas, user);
		await user.click(await body.findByRole("menuitem", { name: "Edit roles" }));
		const dialog = await body.findByRole("dialog");
		await user.click(
			within(dialog).getByLabelText("Auditor", { exact: false }),
		);
		await user.click(within(dialog).getByRole("button", { name: "Confirm" }));
		await body.findByText(/Error updating user roles/);
	},
};

async function openUserMenu(
	canvas: ReturnType<typeof within>,
	user: ReturnType<typeof userEvent.setup>,
	target: User = MockUsers[0],
) {
	const row = canvas.getByRole("button", {
		name: (accessibleName: string) => accessibleName.includes(target.email),
	});
	await user.click(within(row).getByRole("button", { name: /open menu/i }));
}

function replaceUser(users: User[], index: number, user: User) {
	return users.map((u, i) => (i === index ? { ...u, ...user } : u));
}
