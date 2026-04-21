import type { Meta, StoryObj } from "@storybook/react-vite";
import { action } from "storybook/actions";
import { userEvent, within } from "storybook/test";
import {
	MockOrganization,
	MockOrganization2,
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
