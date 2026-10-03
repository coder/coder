import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, spyOn, userEvent, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { API } from "#/api/api";
import { deploymentConfigQueryKey } from "#/api/queries/deployment";
import { userKey } from "#/api/queries/users";
import type { User } from "#/api/typesGenerated";
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

const renamedUsername = "renamed-user";
const renamedName = "Renamed User";
const renamedUser: User = {
	...MockUserMember,
	username: renamedUsername,
	name: renamedName,
};

export const SavesAndRenames: Story = {
	beforeEach: () => {
		spyOn(API, "updateProfile").mockResolvedValue(renamedUser);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await canvas.findByRole("heading", { name: `Edit ${MockUserMember.name}` });

		const username = canvas.getByLabelText(/username/i);
		await userEvent.clear(username);
		await userEvent.type(username, renamedUsername);
		const name = canvas.getByLabelText("Name");
		await userEvent.clear(name);
		await userEvent.type(name, renamedName);
		await userEvent.click(canvas.getByRole("button", { name: /save/i }));

		// The saved user comes from the mutation response, so the heading updates
		// without a refetch.
		await expect(
			await canvas.findByRole("heading", { name: `Edit ${renamedName}` }),
		).toBeVisible();
		await expect(username).toHaveValue(renamedUsername);
		expect(API.updateProfile).toHaveBeenCalledWith(MockUserMember.id, {
			username: renamedUsername,
			name: renamedName,
			avatar_url: "",
		});
	},
};

const suspendedUser: User = { ...MockUserMember, status: "suspended" };
// The refetched profile differs from the one the page loaded, as it would if
// another admin renamed the user in the meantime.
const refetchedUser: User = { ...suspendedUser, name: "Renamed Elsewhere" };

// Suspending invalidates the user query. The refetched user must not replace
// what the admin has typed into the form.
export const KeepsDraftWhenUserRefetches: Story = {
	beforeEach: () => {
		spyOn(API, "suspendUser").mockResolvedValue(suspendedUser);
		spyOn(API, "getUser").mockResolvedValue(refetchedUser);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await canvas.findByRole("heading", { name: `Edit ${MockUserMember.name}` });
		const name = canvas.getByLabelText("Name");
		await userEvent.clear(name);
		await userEvent.type(name, "Draft Name");

		await userEvent.click(canvas.getByRole("button", { name: /open menu/i }));
		const body = within(document.body);
		await userEvent.click(
			await body.findByRole("menuitem", { name: "Suspend…" }),
		);
		const dialog = await body.findByRole("dialog");
		await userEvent.click(
			within(dialog).getByRole("button", { name: "Suspend" }),
		);

		// The success toast is shown after the invalidated user query settles.
		await body.findByText(
			`User "${MockUserMember.username}" suspended successfully.`,
		);
		expect(API.getUser).toHaveBeenCalledWith(MockUserMember.username);
		await expect(name).toHaveValue("Draft Name");
		await expect(
			await canvas.findByRole("heading", {
				name: `Edit ${refetchedUser.name}`,
			}),
		).toBeVisible();
	},
};

export const DeletesAndReturnsToUsers: Story = {
	beforeEach: () => {
		spyOn(API, "deleteUser").mockResolvedValue();
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await canvas.findByRole("heading", { name: `Edit ${MockUserMember.name}` });
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
