import type { Meta, StoryObj } from "@storybook/react";
import { MockUserOwner } from "testHelpers/entities";
import { ResetPasswordDialog } from "./ResetPasswordDialog";

const meta: Meta<typeof ResetPasswordDialog> = {
	title: "pages/UsersPage/ResetPasswordDialog",
	component: ResetPasswordDialog,
};

export default meta;
type Story = StoryObj<typeof ResetPasswordDialog>;

const Example: Story = {
	args: {
		open: true,
		user: MockUserOwner,
		newPassword: "somerandomstringhere",
		onConfirm: () => {},
		onClose: () => {},
	},
};

export { Example as ResetPasswordDialog };
