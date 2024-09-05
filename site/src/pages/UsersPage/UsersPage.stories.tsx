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

const meta: Meta<typeof UsersPage> = {
	title: "pages/UsersPage",
	component: UsersPage,
	parameters: {
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
	},
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

	// Click on "Suspend..."
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
