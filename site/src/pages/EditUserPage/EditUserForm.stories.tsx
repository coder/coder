import type { Meta, StoryObj } from "@storybook/react-vite";
import { action } from "storybook/actions";
import { expect, userEvent, within } from "storybook/test";
import { UserMoreActions } from "#/modules/users/UserMoreActions";
import {
	MockUserMember,
	MockUserOwner,
	mockApiError,
} from "#/testHelpers/entities";
import { docs } from "#/utils/docs";
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

export const Ready: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("heading", { name: "Edit John Doe" }),
		).toBeVisible();
		await expect(
			canvas.getByRole("link", { name: /back to users/i }),
		).toBeVisible();
		await expect(
			canvas.getByRole("link", { name: /view docs/i }),
		).toHaveAttribute("href", docs("/admin/users#edit-a-users-profile"));
		await expect(canvas.getByText("Unique identifier.")).toBeVisible();
		await expect(
			canvas.getByText("Friendly name. Defaults to the username if blank."),
		).toBeVisible();
	},
};

export const NoDisplayName: Story = {
	args: {
		initialValues: {
			username: "jane-doe",
			name: "",
			avatar_url: "",
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("heading", { name: "Edit jane-doe" }),
		).toBeVisible();
	},
};

// The heading names the saved user, so editing the name field must not rename
// the page while the admin is still typing.
export const HeadingKeepsSavedName: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const field = canvas.getByLabelText("Name");
		await userEvent.clear(field);
		await userEvent.type(field, "Jonathan Doe");
		await expect(field).toHaveValue("Jonathan Doe");
		await expect(
			canvas.getByRole("heading", { name: "Edit John Doe" }),
		).toBeVisible();
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
		await expect(canvas.getByLabelText(/username/i)).toBeVisible();
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
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button", { name: /save/i }));
		await expect(
			await canvas.findByText("Username is already taken."),
		).toBeVisible();
		await expect(canvas.queryByRole("alert")).not.toBeInTheDocument();
	},
};

export const GeneralError: Story = {
	args: {
		error: mockApiError({
			message: "Failed to update user profile.",
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByText("Failed to update user profile."),
		).toBeVisible();
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

export const WithActionsMenu: Story = {
	args: {
		headerActions: (
			<UserMoreActions
				user={MockUserMember}
				me={MockUserOwner.id}
				showEdit={false}
				canViewActivity
				onAction={action("action")}
			/>
		),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("link", { name: /back to users/i }),
		).toBeVisible();
		await userEvent.click(canvas.getByRole("button", { name: /open menu/i }));
		const menu = within(document.body);
		await menu.findByRole("menuitem", { name: "View workspaces" });
		await expect(
			menu.queryByRole("menuitem", { name: "Edit" }),
		).not.toBeInTheDocument();
		await menu.findByRole("menuitem", { name: "Suspend…" });
	},
};
