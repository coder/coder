import type { Meta, StoryObj } from "@storybook/react-vite";
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
	args: {},
};

export default meta;
type Story = StoryObj<typeof UsersTable>;

export const Example: Story = {
	args: {
		users: [MockUserOwner, MockUserMember],
		canEditUsers: false,
		groupsByUserId: mockGroupsByUserId,
	},
};

export const Editable: Story = {
	args: {
		users: [
			MockUserOwner,
			MockUserMember,
			{
				...MockUserOwner,
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
				username: "Roger Moore",
				email: "roger.moore@coder.com",
				roles: [],
				status: "suspended",
			},
			{
				...MockUserOwner,
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
