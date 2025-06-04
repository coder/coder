import type { Meta, StoryObj } from "@storybook/react";
import { InboxButton } from "./InboxButton";

const meta: Meta<typeof InboxButton> = {
	title: "modules/notifications/NotificationsInbox/InboxButton",
	component: InboxButton,
};

export default meta;
type Story = StoryObj<typeof InboxButton>;

export const AllRead: Story = {};

export const Unread: Story = {
	args: {
		unreadCount: 3,
	},
};
