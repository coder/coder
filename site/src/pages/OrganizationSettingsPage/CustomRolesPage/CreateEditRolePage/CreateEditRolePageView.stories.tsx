import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import {
	assignableRole,
	MockRole2WithOrgPermissions,
	MockRoleWithOrgPermissions,
	mockApiError,
} from "#/testHelpers/entities";
import CreateEditRolePageView from "./CreateEditRolePageView";

const meta: Meta<typeof CreateEditRolePageView> = {
	title:
		"pages/OrganizationSettingsPage/CustomRolesPage/CreateEditRolePage/CreateEditRolePageView",
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
		const checkbox = canvas.getByRole("checkbox", { name: "audit_log" });
		await user.click(checkbox);
		await expect(checkbox).toBeChecked();
		await user.click(checkbox);
		await expect(checkbox).not.toBeChecked();
	},
};

export const ToggleAdvancedPermissions: Story = {
	args: {
		...Default.args,
		role: undefined,
	},
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const canvas = within(canvasElement);

		// Advanced-only resources are hidden until the switch is enabled.
		expect(
			canvas.queryByRole("checkbox", { name: "api_key" }),
		).not.toBeInTheDocument();

		const [toggle] = canvas.getAllByRole("switch", {
			name: "Show advanced permissions",
		});
		await user.click(toggle);

		// Enabling the switch reveals advanced resources and flips the label.
		await expect(
			canvas.getByRole("checkbox", { name: "api_key" }),
		).toBeInTheDocument();
		const [enabledToggle] = canvas.getAllByRole("switch", {
			name: "Hide advanced permissions",
		});
		await expect(enabledToggle).toBeChecked();

		await user.click(enabledToggle);

		// Disabling the switch hides advanced resources again.
		await expect(
			canvas.queryByRole("checkbox", { name: "api_key" }),
		).not.toBeInTheDocument();
		const [disabledToggle] = canvas.getAllByRole("switch", {
			name: "Show advanced permissions",
		});
		await expect(disabledToggle).not.toBeChecked();
	},
};
