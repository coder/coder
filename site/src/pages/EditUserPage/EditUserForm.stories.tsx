import { mockApiError } from "testHelpers/entities";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { action } from "storybook/actions";
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
