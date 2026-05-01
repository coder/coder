import type { Meta, StoryObj } from "@storybook/react-vite";
import { action } from "storybook/actions";
import {
	assignableRole,
	MockAuditorRole,
	MockOwnerRole,
	MockTemplateAdminRole,
	MockUserAdminRole,
	mockApiError,
} from "#/testHelpers/entities";
import { RoleSelector } from "./RoleSelector";

const meta: Meta<typeof RoleSelector> = {
	title: "pages/CreateUserPage/RoleSelector",
	component: RoleSelector,
	args: {
		onChange: action("change"),
		selectedRoles: [],
	},
};

export default meta;
type Story = StoryObj<typeof RoleSelector>;

const allAssignable = [
	assignableRole(MockOwnerRole, true),
	assignableRole(MockUserAdminRole, true),
	assignableRole(MockTemplateAdminRole, true),
	assignableRole(MockAuditorRole, true),
];

const someNonAssignable = [
	assignableRole(MockOwnerRole, false),
	assignableRole(MockUserAdminRole, true),
	assignableRole(MockTemplateAdminRole, false),
	assignableRole(MockAuditorRole, true),
];

export const Default: Story = {
	args: {
		roles: allAssignable,
	},
};

export const WithSelections: Story = {
	args: {
		roles: allAssignable,
		selectedRoles: [MockUserAdminRole.name, MockAuditorRole.name],
	},
};

export const WithNonAssignableRoles: Story = {
	args: {
		roles: someNonAssignable,
	},
};

export const Loading: Story = {
	args: {
		roles: [],
		loading: true,
	},
};

export const WithError: Story = {
	args: {
		roles: [],
		error: mockApiError({ message: "Failed to fetch assignable roles." }),
	},
};
