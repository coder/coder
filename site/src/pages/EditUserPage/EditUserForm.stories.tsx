import type { Meta, StoryObj } from "@storybook/react-vite";
import { action } from "storybook/actions";
import { expect, userEvent, within } from "storybook/test";
import { mockApiError } from "#/testHelpers/entities";
import { EditUserForm } from "./EditUserForm";

const meta: Meta<typeof EditUserForm> = {
	title: "pages/EditUserPage",
	component: EditUserForm,
	args: {
		onCancel: action("cancel"),
		onSubmit: action("submit"),
		isLoading: false,
		canEditAvatar: true,
		initialValues: {
			username: "john-doe",
			name: "John Doe",
			avatar_url: "",
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
			avatar_url: "",
		},
	},
};

export const WithAvatar: Story = {
	args: {
		initialValues: {
			username: "john-doe",
			name: "John Doe",
			avatar_url: "/emojis/1f600.png",
		},
	},
};

export const EditAvatar: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const field = canvas.getByLabelText("Avatar URL");
		await userEvent.clear(field);
		// Typing happens one character at a time, so the value passes through
		// incomplete states like "https:" that must not crash the preview.
		await userEvent.type(field, "https://example.com/avatar.png");
		await expect(field).toHaveValue("https://example.com/avatar.png");
	},
};

// The avatar field is hidden for login types whose avatar is synced from an
// identity provider (e.g. github, oidc).
export const CannotEditAvatar: Story = {
	args: {
		canEditAvatar: false,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.queryByLabelText("Avatar URL")).not.toBeInTheDocument();
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
