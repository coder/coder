import type { Meta, StoryObj } from "@storybook/react";
import { expect, userEvent, within } from "@storybook/test";
import {
	MockRole2WithOrgPermissions,
	MockRoleWithOrgPermissions,
	assignableRole,
	mockApiError,
} from "testHelpers/entities";
import { CreateEditRolePageView } from "./CreateEditRolePageView";

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
		...Default.args,
		role: assignableRole(MockRole2WithOrgPermissions, true),
	},
};

export const WithError: Story = {
	args: {
		...Default.args,
		role: undefined,
		error: "this is an error",
	},
};

export const WithValidationError: Story = {
	args: {
		...Default.args,
		role: undefined,
		error: mockApiError({
			message: "A role named new-role already exists.",
			validations: [{ field: "name", detail: "Role names must be unique" }],
		}),
	},
	play: async ({ canvasElement, step }) => {
		const canvas = within(canvasElement);

		await step("Enter name", async () => {
			const input = canvas.getByLabelText("Name");
			await userEvent.type(input, "new-role");
			input.blur();
		});
	},
};

export const InvalidCharsError: Story = {
	args: {
		...Default.args,
		role: undefined,
	},
	play: async ({ canvasElement, step }) => {
		const canvas = within(canvasElement);

		await step("Enter name", async () => {
			const input = canvas.getByLabelText("Name");
			await userEvent.type(input, "!~@#@!");
			input.blur();
		});
	},
};

export const CannotEditRoleName: Story = {
	args: {
		...Default.args,
		canAssignOrgRole: false,
	},
};

export const ShowAllResources: Story = {
	args: {
		...Default.args,
		allResources: true,
	},
};

export const Loading: Story = {
	args: {
		...Default.args,
		isLoading: true,
	},
};

export const ToggleParentCheckbox: Story = {
	args: {
		...Default.args,
		role: undefined,
	},
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
