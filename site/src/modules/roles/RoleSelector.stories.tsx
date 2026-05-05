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
	title: "modules/roles/RoleSelector",
	component: RoleSelector,
	args: {
		onChange: action("change"),
		selectedRoles: new Set(),
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
		availableRoles: allAssignable,
	},
};

export const WithSelections: Story = {
	args: {
		availableRoles: allAssignable,
		selectedRoles: new Set([MockUserAdminRole.name, MockAuditorRole.name]),
	},
};

export const WithNonAssignableRoles: Story = {
	args: {
		availableRoles: someNonAssignable,
	},
};

export const Loading: Story = {
	args: {
		availableRoles: [],
		loading: true,
	},
};

export const WithError: Story = {
	args: {
		availableRoles: [],
		error: mockApiError({ message: "Failed to fetch assignable roles." }),
	},
};
