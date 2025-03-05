import type { Meta, StoryObj } from "@storybook/react";
import { InboxItem } from "./InboxItem";
import { fn, userEvent, within, expect } from "@storybook/test";
import { MockNotification } from "testHelpers/entities";

const meta: Meta<typeof InboxItem> = {
	title: "modules/notifications/NotificationsInbox/InboxItem",
	component: InboxItem,
	render: (args) => {
		return (
			<div className="max-w-[460px] border-solid border-border rounded">
				<InboxItem {...args} />
			</div>
		);
	},
};

export default meta;
type Story = StoryObj<typeof InboxItem>;

export const Read: Story = {
	args: {
		notification: MockNotification,
	},
};

export const Unread: Story = {
	args: {
		notification: {
			...MockNotification,
			read_status: "unread",
		},
	},
};

export const UnreadFocus: Story = {
	args: {
		notification: {
			...MockNotification,
			read_status: "unread",
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const notification = canvas.getByRole("menuitem");
		await userEvent.click(notification);
	},
};

export const OnMarkNotificationAsRead: Story = {
	args: {
		notification: {
			...MockNotification,
			read_status: "unread",
		},
		onMarkNotificationAsRead: fn(),
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const notification = canvas.getByRole("menuitem");
		await userEvent.click(notification);
		const markButton = canvas.getByRole("button", { name: /mark as read/i });
		await userEvent.click(markButton);
		await expect(args.onMarkNotificationAsRead).toHaveBeenCalledTimes(1);
		await expect(args.onMarkNotificationAsRead).toHaveBeenCalledWith(
			args.notification.id,
		);
	},
	parameters: {
		chromatic: {
			disableSnapshot: true,
		},
	},
};
