import type { Meta, StoryObj } from "@storybook/react-vite";
import { type ComponentProps, type FC, useState } from "react";
import { expect, fn, spyOn, userEvent, within } from "storybook/test";
import { API } from "#/api/api";
import {
	MockUserMember,
	MockUserOwner,
	SuspendedMockUser,
} from "#/testHelpers/entities";
import { withToaster } from "#/testHelpers/storybook";
import { UserActionDialogs, type UserAdminAction } from "./UserActionDialogs";
import { UserMoreActions } from "./UserMoreActions";

const UserMoreActionsWithDialogs: FC<
	ComponentProps<typeof UserMoreActions>
> = ({ onAction, ...props }) => {
	const [action, setAction] = useState<UserAdminAction>();

	return (
		<>
			<UserMoreActions
				{...props}
				onAction={(next) => {
					onAction(next);
					setAction(next);
				}}
			/>
			<UserActionDialogs action={action} onClose={() => setAction(undefined)} />
		</>
	);
};

const meta: Meta<typeof UserMoreActions> = {
	title: "modules/users/UserMoreActions",
	component: UserMoreActions,
	decorators: [withToaster],
	args: {
		user: MockUserMember,
		me: MockUserOwner.id,
		canViewActivity: true,
		onAction: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof UserMoreActions>;

export const OpenMenu: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button", { name: /open menu/i }));
		const menu = within(document.body);
		await menu.findByRole("menuitem", { name: "View workspaces" });
		await menu.findByRole("menuitem", { name: "View activity" });
		await menu.findByRole("menuitem", { name: "Edit" });
		await menu.findByRole("menuitem", { name: "Edit roles" });
		await menu.findByRole("menuitem", { name: "Suspend…" });
		await menu.findByRole("menuitem", { name: "Delete…" });
	},
};

export const OidcRoleSyncDisablesEditRoles: Story = {
	args: {
		user: { ...MockUserMember, login_type: "oidc" },
		oidcRoleSyncEnabled: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button", { name: /open menu/i }));
		const menu = within(document.body);
		await expect(
			await menu.findByRole("menuitem", { name: "Edit roles" }),
		).toHaveAttribute("aria-disabled", "true");
	},
};

export const WithoutActivity: Story = {
	args: {
		canViewActivity: false,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button", { name: /open menu/i }));
		const menu = within(document.body);
		await menu.findByRole("menuitem", { name: "View workspaces" });
		await expect(
			menu.queryByRole("menuitem", { name: "View activity" }),
		).not.toBeInTheDocument();
	},
};

export const Suspended: Story = {
	args: {
		user: SuspendedMockUser,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button", { name: /open menu/i }));
		const menu = within(document.body);
		await menu.findByRole("menuitem", { name: "Activate…" });
		await expect(
			menu.queryByRole("menuitem", { name: "Suspend…" }),
		).not.toBeInTheDocument();
		await expect(
			menu.queryByRole("menuitem", { name: "Reset password…" }),
		).not.toBeInTheDocument();
	},
};

export const CannotDeleteSelf: Story = {
	args: {
		user: MockUserOwner,
		me: MockUserOwner.id,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button", { name: /open menu/i }));
		await expect(
			await within(document.body).findByRole("menuitem", { name: "Delete…" }),
		).toHaveAttribute("aria-disabled", "true");
	},
};

export const OpensSuspendDialog: Story = {
	render: (args) => <UserMoreActionsWithDialogs {...args} />,
	beforeEach: () => {
		spyOn(API, "suspendUser").mockResolvedValue({
			...MockUserMember,
			status: "suspended",
		});
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button", { name: /open menu/i }));
		await userEvent.click(
			await within(document.body).findByRole("menuitem", { name: "Suspend…" }),
		);
		const dialog = await within(document.body).findByRole("dialog");
		await within(dialog).findByRole("heading", { name: "Suspend user" });
		await userEvent.click(
			within(dialog).getByRole("button", { name: "Suspend" }),
		);
		await within(document.body).findByText(/suspended successfully/);
	},
};
