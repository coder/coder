import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import {
	MockAuditorRole,
	MockGroup,
	MockMemberRole,
	MockTemplateAdminRole,
	MockUserAdminRole,
	MockUserMember,
	MockUserOwner,
} from "#/testHelpers/entities";
import { UsersTable } from "./UsersTable";

const mockGroupsByUserId = new Map([
	[MockUserOwner.id, [MockGroup]],
	[MockUserMember.id, [MockGroup]],
]);

const meta: Meta<typeof UsersTable> = {
	title: "pages/UsersPage/UsersTable",
	component: UsersTable,
	args: {
		me: MockUserOwner.id,
		onAction: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof UsersTable>;

export const Example: Story = {
	args: {
		users: [MockUserOwner, MockUserMember],
		canEditUsers: false,
		groupsByUserId: mockGroupsByUserId,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByRole("table", { name: "Users" })).toBeVisible();
		await expect(canvas.getByText(MockUserOwner.username)).toBeVisible();
		await expect(
			canvas.queryByRole("button", { name: /open menu/i }),
		).not.toBeInTheDocument();
	},
};

export const Editable: Story = {
	args: {
		users: [
			MockUserOwner,
			MockUserMember,
			{
				...MockUserOwner,
				id: "john-doe",
				username: "John Doe",
				email: "john.doe@coder.com",
				roles: [
					MockUserAdminRole,
					MockTemplateAdminRole,
					MockMemberRole,
					MockAuditorRole,
				],
				status: "dormant",
			},
			{
				...MockUserOwner,
				id: "roger-moore",
				username: "Roger Moore",
				email: "roger.moore@coder.com",
				roles: [],
				status: "suspended",
			},
			{
				...MockUserOwner,
				id: "oidc-user",
				username: "OIDC User",
				email: "oidc.user@coder.com",
				roles: [],
				status: "active",
				login_type: "oidc",
			},
		],
		canEditUsers: true,
		canViewActivity: true,
		groupsByUserId: mockGroupsByUserId,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const ownerRow = canvas.getByRole("row", {
			name: (accessibleName) => accessibleName.includes(MockUserOwner.email),
		});
		await userEvent.click(
			within(ownerRow).getByRole("button", { name: /open menu/i }),
		);
		const menu = within(document.body);
		await menu.findByRole("menuitem", { name: "Edit" });
		await menu.findByRole("menuitem", { name: "Edit roles" });
	},
};

export const Empty: Story = {
	args: {
		users: [],
	},
};

export const Loading: Story = {
	args: {
		users: [],
		isLoading: true,
	},
};
