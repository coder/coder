import type { Meta, StoryObj } from "@storybook/react-vite";
import { action } from "storybook/actions";
import {
	MockAssignableSiteRoles,
	MockAuditorRole,
	MockOwnerRole,
	MockUserAdminRole,
	MockUserMember,
	MockUserOwner,
	mockApiError,
} from "#/testHelpers/entities";
import { EditUserForm } from "./EditUserForm";

const meta: Meta<typeof EditUserForm> = {
	title: "pages/EditUserPage",
	component: EditUserForm,
	args: {
		onCancel: action("cancel"),
		onSubmit: action("submit"),
		isLoading: false,
		initialValues: {
			username: "john-doe",
			name: "John Doe",
		},
	},
};

export default meta;
type Story = StoryObj<typeof EditUserForm>;

export const Ready: Story = {};

export const NoDisplayName: Story = {
	args: {
		initialValues: {
			username: "jane-doe",
			name: "",
		},
	},
};

export const FormError: Story = {
	args: {
		error: mockApiError({
			validations: [
				{ field: "username", detail: "Username is already taken." },
			],
		}),
	},
};

export const GeneralError: Story = {
	args: {
		error: mockApiError({
			message: "Failed to update user profile.",
		}),
	},
};

export const Loading: Story = {
	args: {
		isLoading: true,
	},
};

export const WithRoles: Story = {
	args: {
		user: {
			...MockUserOwner,
			roles: [MockUserAdminRole, MockAuditorRole],
		},
		availableRoles: MockAssignableSiteRoles,
		canEditRoles: true,
		oidcRoleSyncEnabled: false,
		isUpdatingRoles: false,
		onUpdateRoles: action("updateRoles"),
		initialValues: {
			username: MockUserOwner.username,
			name: MockUserOwner.name ?? "",
		},
	},
};

export const WithRolesMemberOnly: Story = {
	args: {
		user: MockUserMember,
		availableRoles: MockAssignableSiteRoles,
		canEditRoles: true,
		oidcRoleSyncEnabled: false,
		isUpdatingRoles: false,
		onUpdateRoles: action("updateRoles"),
		initialValues: {
			username: MockUserMember.username,
			name: MockUserMember.name ?? "",
		},
	},
};

export const WithRolesOIDCSync: Story = {
	args: {
		user: {
			...MockUserMember,
			login_type: "oidc",
			roles: [MockOwnerRole],
		},
		availableRoles: MockAssignableSiteRoles,
		canEditRoles: true,
		oidcRoleSyncEnabled: true,
		isUpdatingRoles: false,
		onUpdateRoles: action("updateRoles"),
		initialValues: {
			username: MockUserMember.username,
			name: MockUserMember.name ?? "",
		},
	},
};

export const WithRolesNoPermission: Story = {
	args: {
		user: MockUserOwner,
		availableRoles: MockAssignableSiteRoles,
		canEditRoles: false,
		initialValues: {
			username: MockUserOwner.username,
			name: MockUserOwner.name ?? "",
		},
	},
};
