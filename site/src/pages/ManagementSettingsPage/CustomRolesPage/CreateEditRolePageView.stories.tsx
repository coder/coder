import type { Meta, StoryObj } from "@storybook/react";
import {
	MockRoleWithOrgPermissions,
	MockRole2WithOrgPermissions,
	assignableRole,
	mockApiError,
} from "testHelpers/entities";
import { CreateEditRolePageView } from "./CreateEditRolePageView";
import { userEvent, within, expect } from "@storybook/test";

const meta: Meta<typeof CreateEditRolePageView> = {
	title: "pages/OrganizationCreateEditRolePage",
	component: CreateEditRolePageView,
};

export default meta;
type Story = StoryObj<typeof CreateEditRolePageView>;

export const Default: Story = {
	args: {
		role: assignableRole(MockRoleWithOrgPermissions, true),
		onSubmit: () => null,
		error: undefined,
		isLoading: false,
		organizationName: "my-org",
		canAssignOrgRole: true,
	},
};

export const CheckboxIndeterminate: Story = {
	args: {
		role: assignableRole(MockRole2WithOrgPermissions, true),
		onSubmit: () => null,
		isLoading: false,
		organizationName: "my-org",
		canAssignOrgRole: true,
	},
};

export const WithError: Story = {
	args: {
		role: assignableRole(MockRoleWithOrgPermissions, true),
		onSubmit: () => null,
		error: mockApiError({
			message: "A role named new-role already exists.",
			validations: [{ field: "name", detail: "Role names must be unique" }],
		}),
		isLoading: false,
		organizationName: "my-org",
		canAssignOrgRole: true,
	},
};

export const CannotEdit: Story = {
	args: {
		role: assignableRole(MockRoleWithOrgPermissions, true),
		onSubmit: () => null,
		error: undefined,
		isLoading: false,
		organizationName: "my-org",
		canAssignOrgRole: false,
	},
};

export const ShowAllResources: Story = {
	args: {
		role: assignableRole(MockRoleWithOrgPermissions, true),
		onSubmit: () => null,
		error: undefined,
		isLoading: false,
		organizationName: "my-org",
		canAssignOrgRole: true,
		allResources: true,
	},
};

export const ToggleParentCheckbox: Story = {
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		const checkbox = await canvas
			.getByTestId("audit_log")
			.getElementsByTagName("input")[0];
		await user.click(checkbox);
		await expect(checkbox).toBeChecked();
		await user.click(checkbox);
		await expect(checkbox).not.toBeChecked();
	},
};
