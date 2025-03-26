import type { Meta, StoryObj } from "@storybook/react";
import { InboxAvatar } from "./InboxAvatar";

const meta: Meta<typeof InboxAvatar> = {
	title: "modules/notifications/NotificationsInbox/InboxAvatar",
	component: InboxAvatar,
};

export default meta;
type Story = StoryObj<typeof InboxAvatar>;

export const Workspace: Story = {
	args: {
		icon: "DEFAULT_WORKSPACE_ICON",
	},
};

export const Account: Story = {
	args: {
		icon: "DEFAULT_ACCOUNT_ICON",
	},
};

export const Template: Story = {
	args: {
		icon: "DEFAULT_TEMPLATE_ICON",
	},
};

export const Other: Story = {
	args: {
		icon: "DEFAULT_OTHER_ICON",
	},
};
