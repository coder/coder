import type { Meta, StoryObj } from "@storybook/react-vite";
import { action } from "storybook/actions";
import { userEvent, within } from "storybook/test";
import {
	assignableRole,
	MockAuditorRole,
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
		serviceAccountsEnabled: true,
	},
};

export default meta;
type Story = StoryObj<typeof CreateUserForm>;

export const Ready: Story = {};

// Query key used by permittedOrganizations() in the form.
const permittedOrgsKey = [
	"organizations",
	"permitted",
	{ object: { resource_type: "organization_member" }, action: "create" },
];

export const WithOrganizations: Story = {
	parameters: {
		queries: [
			{
				key: permittedOrgsKey,
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
};

export const GeneralError: Story = {
	args: {
		error: mockApiError({
			message: "User already exists",
		}),
	},
};

export const Loading: Story = {
	args: {
		isLoading: true,
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
		authMethods: MockAuthMethodsPasswordOnly,
	},
};

export const RolesError: Story = {
	args: {
		rolesError: mockApiError({
			message: "Failed to fetch assignable roles.",
		}),
		authMethods: MockAuthMethodsPasswordOnly,
	},
};

export const WithRoles: Story = {
	args: {
		availableRoles: mockAvailableRoles,
		authMethods: MockAuthMethodsPasswordOnly,
	},
};

export const WithRolesSelected: Story = {
	args: {
		availableRoles: mockAvailableRoles,
		authMethods: MockAuthMethodsPasswordOnly,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("checkbox", { name: /owner/i }));
		await userEvent.click(canvas.getByRole("checkbox", { name: /auditor/i }));
	},
};
