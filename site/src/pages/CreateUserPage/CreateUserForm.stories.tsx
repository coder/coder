import type { Meta, StoryObj } from "@storybook/react-vite";
import { action } from "storybook/actions";
import { expect, userEvent, within } from "storybook/test";
import { permittedOrganizationsKey } from "#/api/queries/organizations";
import {
	assignableRole,
	MockAuditorRole,
	MockAuthMethodsAll,
	MockAuthMethodsPasswordOnly,
	MockOrganization,
	MockOrganization2,
	MockOwnerRole,
	MockTemplateAdminRole,
	MockUserAdminRole,
	mockApiError,
} from "#/testHelpers/entities";
import { CreateUserForm } from "./CreateUserForm";

const meta: Meta<typeof CreateUserForm> = {
	title: "pages/CreateUserPage",
	component: CreateUserForm,
	args: {
		onCancel: action("cancel"),
		onSubmit: action("submit"),
		isLoading: false,
		showOrganizations: false,
		serviceAccountsEnabled: true,
		authMethods: MockAuthMethodsPasswordOnly,
	},
};

export default meta;
type Story = StoryObj<typeof CreateUserForm>;

export const Ready: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("heading", { name: "New user" }),
		).toBeVisible();
		await expect(
			canvas.getByRole("link", { name: /back to users/i }),
		).toBeVisible();
		await expect(
			canvas.getByText(/add a user to this coder deployment/i),
		).toBeVisible();
		await expect(canvas.getByText("Unique identifier.")).toBeVisible();
		await expect(
			canvas.getByText("Friendly name. Defaults to the username if blank."),
		).toBeVisible();
		await expect(canvas.queryByText(/optional/i)).not.toBeInTheDocument();
	},
};

export const NoAvailableLoginTypes: Story = {
	args: {
		authMethods: {
			password: { enabled: false },
			github: { enabled: false, default_provider_configured: false },
			oidc: { enabled: false, signInText: "", iconUrl: "" },
		},
		serviceAccountsEnabled: false,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByText(
				"No authentication methods are available for new users.",
			),
		).toBeVisible();
		await expect(
			canvas.queryByRole("button", { name: /save/i }),
		).not.toBeInTheDocument();
	},
};

export const KeepsEmailForExternalLogin: Story = {
	args: {
		authMethods: MockAuthMethodsAll,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const email = canvas.getByLabelText(/email/i);
		await userEvent.type(email, "someone@coder.com");
		await userEvent.click(canvas.getByTestId("login-type-input"));
		await userEvent.click(
			await within(document.body).findByRole("option", {
				name: /openID connect/i,
			}),
		);
		await expect(email).toHaveValue("someone@coder.com");
	},
};

export const WithOrganizations: Story = {
	parameters: {
		queries: [
			{
				key: permittedOrganizationsKey({
					object: { resource_type: "organization_member" },
					action: "create",
				}),
				data: [MockOrganization, MockOrganization2],
			},
		],
	},
	args: {
		showOrganizations: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByLabelText("Organization"));
	},
};

export const FormError: Story = {
	args: {
		error: mockApiError({
			validations: [{ field: "username", detail: "Username taken" }],
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button", { name: /save/i }));
		await expect(await canvas.findByText("Username taken")).toBeVisible();
		await expect(canvas.queryByRole("alert")).not.toBeInTheDocument();
	},
};

export const GeneralError: Story = {
	args: {
		error: mockApiError({
			message: "User already exists",
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("User already exists")).toBeVisible();
		await expect(canvas.getByLabelText(/username/i)).toBeVisible();
	},
};

export const Loading: Story = {
	args: {
		isLoading: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByRole("button", { name: /save/i })).toBeDisabled();
		await expect(canvas.getByRole("button", { name: /cancel/i })).toBeEnabled();
	},
};

const mockAvailableRoles = [
	assignableRole(MockOwnerRole, true),
	assignableRole(MockUserAdminRole, true),
	assignableRole(MockTemplateAdminRole, true),
	assignableRole(MockAuditorRole, true),
];

export const RolesLoading: Story = {
	args: {
		rolesLoading: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Roles")).toBeVisible();
		await expect(
			canvas.queryByRole("checkbox", { name: /owner/i }),
		).not.toBeInTheDocument();
	},
};

export const RolesError: Story = {
	args: {
		rolesError: mockApiError({
			message: "Failed to fetch assignable roles.",
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByText("Failed to fetch assignable roles."),
		).toBeVisible();
	},
};

export const WithRoles: Story = {
	args: {
		availableRoles: mockAvailableRoles,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("checkbox", { name: /owner/i }),
		).toBeVisible();
	},
};

export const WithRolesSelected: Story = {
	args: {
		availableRoles: mockAvailableRoles,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("checkbox", { name: /owner/i }));
		await userEvent.click(canvas.getByRole("checkbox", { name: /auditor/i }));
		await expect(
			canvas.getByRole("checkbox", { name: /owner/i }),
		).toBeChecked();
		await expect(
			canvas.getByRole("checkbox", { name: /auditor/i }),
		).toBeChecked();
	},
};
