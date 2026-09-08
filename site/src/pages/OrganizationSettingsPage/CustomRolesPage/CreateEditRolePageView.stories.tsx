import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import {
	assignableRole,
	MockRole2WithOrgPermissions,
	MockRoleWithOrgPermissions,
	mockApiError,
} from "#/testHelpers/entities";
import { CreateEditRolePageView } from "./CreateEditRolePageView";

const organizationName = "my-org";

const meta: Meta<typeof CreateEditRolePageView> = {
	title: "pages/OrganizationCreateEditRolePage",
	component: CreateEditRolePageView,
	args: {
		onSubmit: fn(),
		error: undefined,
		isLoading: false,
		organizationName,
	},
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: `/organizations/${organizationName}/roles/create` },
			routing: [
				{ path: "/organizations/:organization/roles", useStoryElement: true },
				{
					path: "/organizations/:organization/roles/create",
					useStoryElement: true,
				},
				{
					path: "/organizations/:organization/roles/:roleName",
					useStoryElement: true,
				},
			],
		}),
	},
};

export default meta;
type Story = StoryObj<typeof CreateEditRolePageView>;

export const Create: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("heading", { name: "New Custom Role" }),
		).toBeVisible();
		await expect(
			canvas.getByRole("link", { name: /back to roles/i }),
		).toHaveAttribute("href", `/organizations/${organizationName}/roles`);
		await expect(canvas.getByRole("link", { name: "Cancel" })).toHaveAttribute(
			"href",
			`/organizations/${organizationName}/roles`,
		);
		await expect(
			canvas.getByRole("button", { name: "Create custom role" }),
		).toBeEnabled();
	},
};

export const Edit: Story = {
	args: {
		role: assignableRole(MockRoleWithOrgPermissions, true),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("heading", { name: "Edit Custom Role" }),
		).toBeVisible();
		await expect(canvas.getByLabelText(/^Name/)).toBeDisabled();
		await expect(canvas.getByRole("button", { name: "Save" })).toBeEnabled();
	},
};

export const CheckboxIndeterminate: Story = {
	args: {
		role: assignableRole(MockRole2WithOrgPermissions, true),
	},
};

export const WithError: Story = {
	args: {
		error: mockApiError({
			message: "Failed to create custom role.",
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByRole("alert")).toHaveTextContent(
			"Failed to create custom role.",
		);
	},
};

export const WithValidationError: Story = {
	args: {
		error: mockApiError({
			message: "A role named new-role already exists.",
			validations: [{ field: "name", detail: "Role names must be unique" }],
		}),
	},
	play: async ({ canvasElement, step }) => {
		const canvas = within(canvasElement);

		await step("Enter name", async () => {
			const input = canvas.getByLabelText(/^Name/);
			await userEvent.type(input, "new-role");
			input.blur();
		});
	},
};

export const InvalidCharsError: Story = {
	play: async ({ canvasElement, step }) => {
		const canvas = within(canvasElement);

		await step("Enter name", async () => {
			const input = canvas.getByLabelText(/^Name/);
			await userEvent.type(input, "!~@#@!");
			input.blur();
		});
	},
};

export const ShowAllResources: Story = {
	args: {
		role: assignableRole(MockRoleWithOrgPermissions, true),
		allResources: true,
	},
};

export const Loading: Story = {
	args: {
		role: assignableRole(MockRoleWithOrgPermissions, true),
		isLoading: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByRole("button", { name: /save/i })).toBeDisabled();
	},
};

export const ToggleParentCheckbox: Story = {
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
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const canvas = within(canvasElement);

		expect(
			canvas.queryByRole("checkbox", { name: "api_key" }),
		).not.toBeInTheDocument();

		const toggle = canvas.getByRole("switch", {
			name: "Show advanced permissions",
		});
		await user.click(toggle);

		await expect(
			canvas.getByRole("checkbox", { name: "api_key" }),
		).toBeInTheDocument();
		const enabledToggle = canvas.getByRole("switch", {
			name: "Hide advanced permissions",
		});
		await expect(enabledToggle).toBeChecked();

		await user.click(enabledToggle);

		await expect(
			canvas.queryByRole("checkbox", { name: "api_key" }),
		).not.toBeInTheDocument();
		const disabledToggle = canvas.getByRole("switch", {
			name: "Show advanced permissions",
		});
		await expect(disabledToggle).not.toBeChecked();
	},
};

export const SubmitsNewRole: Story = {
	play: async ({ canvasElement, args }) => {
		const user = userEvent.setup();
		const canvas = within(canvasElement);

		await user.type(canvas.getByLabelText(/^Name/), "auditor");
		const memberCheckbox = canvas.getByRole("checkbox", {
			name: "organization_member",
		});
		await user.click(memberCheckbox);
		await expect(memberCheckbox).toBeChecked();
		await user.click(
			canvas.getByRole("button", { name: "Create custom role" }),
		);

		await expect(args.onSubmit).toHaveBeenCalled();
	},
};
