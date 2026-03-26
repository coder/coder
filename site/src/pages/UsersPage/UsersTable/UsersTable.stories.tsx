import type { Meta, StoryObj } from "@storybook/react-vite";
import {
	MockAssignableSiteRoles,
	MockAuditorRole,
	MockAuthMethodsPasswordOnly,
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
		isNonInitialPage: false,
		authMethods: MockAuthMethodsPasswordOnly,
	},
};

export default meta;
type Story = StoryObj<typeof UsersTable>;

export const Example: Story = {
	args: {
		users: [
			{ ...MockUserOwner, has_ai_seat: false },
			{ ...MockUserMember, has_ai_seat: false },
		],
		roles: MockAssignableSiteRoles,
		canEditUsers: false,
		groupsByUserId: mockGroupsByUserId,
	},
};

export const ExampleWithAISeatColumn: Story = {
	args: {
		users: [
			{ ...MockUserOwner, has_ai_seat: true },
			{ ...MockUserMember, has_ai_seat: false },
		],
		roles: MockAssignableSiteRoles,
		canEditUsers: false,
		groupsByUserId: mockGroupsByUserId,
		showAISeatColumn: true,
	},
};

export const Editable: Story = {
	args: {
		users: [
			{ ...MockUserOwner, has_ai_seat: false },
			{ ...MockUserMember, has_ai_seat: false },
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
				has_ai_seat: false,
			},
			{
				...MockUserOwner,
				username: "Roger Moore",
				email: "roger.moore@coder.com",
				roles: [],
				status: "suspended",
				has_ai_seat: false,
			},
			{
				...MockUserOwner,
				username: "OIDC User",
				email: "oidc.user@coder.com",
				roles: [],
				status: "active",
				login_type: "oidc",
				has_ai_seat: false,
			},
		],
		roles: MockAssignableSiteRoles,
		canEditUsers: true,
		canViewActivity: true,
		groupsByUserId: mockGroupsByUserId,
	},
};

export const EditableWithAISeatColumn: Story = {
	args: {
		users: [
			{ ...MockUserOwner, has_ai_seat: true },
			{ ...MockUserMember, has_ai_seat: false },
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
				has_ai_seat: false,
			},
			{
				...MockUserOwner,
				username: "Roger Moore",
				email: "roger.moore@coder.com",
				roles: [],
				status: "suspended",
				has_ai_seat: false,
			},
			{
				...MockUserOwner,
				username: "OIDC User",
				email: "oidc.user@coder.com",
				roles: [],
				status: "active",
				login_type: "oidc",
				has_ai_seat: false,
			},
		],
		roles: MockAssignableSiteRoles,
		canEditUsers: true,
		canViewActivity: true,
		groupsByUserId: mockGroupsByUserId,
		showAISeatColumn: true,
	},
};

export const Empty: Story = {
	args: {
		users: [],
		roles: MockAssignableSiteRoles,
	},
};

export const Loading: Story = {
	args: {
		users: [],
		roles: MockAssignableSiteRoles,
		isLoading: true,
	},
	parameters: {
		chromatic: { pauseAnimationAtEnd: true },
	},
};
