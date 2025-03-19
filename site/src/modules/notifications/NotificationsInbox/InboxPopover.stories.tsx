import type { Meta, StoryObj } from "@storybook/react";
import { expect, fn, userEvent, waitFor, within } from "@storybook/test";
import { MockNotifications } from "testHelpers/entities";
import { InboxPopover } from "./InboxPopover";

const meta: Meta<typeof InboxPopover> = {
	title: "modules/notifications/NotificationsInbox/InboxPopover",
	component: InboxPopover,
	args: {
		defaultOpen: true,
	},
	render: (args) => {
		return (
			<div className="w-full max-w-screen-xl py-2 px-6">
				<header className="flex justify-end">
					<InboxPopover {...args} />
				</header>
			</div>
		);
	},
	parameters: {
		layout: "fullscreen",
		chromatic: {
			desktop: { name: "Custom", styles: { width: "640px", height: "720px" } },
		},
	},
};

export default meta;
type Story = StoryObj<typeof InboxPopover>;

export const Default: Story = {
	args: {
		unreadCount: 2,
		notifications: MockNotifications.slice(0, 3),
	},
};

export const Scrollable: Story = {
	args: {
		unreadCount: 2,
		notifications: MockNotifications,
	},
	play: async ({ canvasElement }) => {
		const body = canvasElement.ownerDocument.body;
		const content = body.querySelector<HTMLDivElement>(
			"[data-radix-scroll-area-viewport]",
		);
		if (!content) {
			throw new Error("ScrollArea content not found");
		}
		await waitFor(() => {
			const distanceToBottom = content?.children[0].clientHeight;
			content.scroll({
				top: distanceToBottom,
			});
			expect(content.scrollTop).not.toBe(0);
		});
	},
};

export const Loading: Story = {
	args: {
		unreadCount: 0,
		notifications: undefined,
	},
};

export const LoadingFailure: Story = {
	args: {
		unreadCount: 0,
		notifications: undefined,
		error: new Error("Failed to load notifications"),
	},
};

export const Empty: Story = {
	args: {
		unreadCount: 0,
		notifications: [],
	},
};

export const OnRetry: Story = {
	args: {
		unreadCount: 0,
		notifications: undefined,
		error: new Error("Failed to load notifications"),
		onRetry: fn(),
	},
	play: async ({ canvasElement, args }) => {
		const body = within(canvasElement.ownerDocument.body);
		const retryButton = body.getByRole("button", { name: /retry/i });
		await userEvent.click(retryButton);
		await expect(args.onRetry).toHaveBeenCalledTimes(1);
	},
	parameters: {
		chromatic: {
			disableSnapshot: true,
		},
	},
};

export const OnMarkAllAsRead: Story = {
	args: {
		defaultOpen: true,
		unreadCount: 2,
		notifications: MockNotifications.slice(0, 3),
		onMarkAllAsRead: fn(),
	},
	play: async ({ canvasElement, args }) => {
		const body = within(canvasElement.ownerDocument.body);
		const markButton = body.getByRole("button", { name: /mark all as read/i });
		await userEvent.click(markButton);
		await expect(args.onMarkAllAsRead).toHaveBeenCalledTimes(1);
	},
	parameters: {
		chromatic: {
			disableSnapshot: true,
		},
	},
};

export const OnMarkNotificationAsRead: Story = {
	args: {
		unreadCount: 2,
		notifications: MockNotifications.slice(0, 3),
		onMarkNotificationAsRead: fn(),
	},
	play: async ({ canvasElement, args }) => {
		const body = within(canvasElement.ownerDocument.body);
		const notifications = body.getAllByRole("menuitem");
		const secondNotification = notifications[1];
		await userEvent.click(secondNotification);
		const markButton = body.getByRole("button", { name: /mark as read/i });
		await userEvent.click(markButton);
		await expect(args.onMarkNotificationAsRead).toHaveBeenCalledTimes(1);
		await expect(args.onMarkNotificationAsRead).toHaveBeenCalledWith(
			args.notifications?.[1].id,
		);
	},
	parameters: {
		chromatic: {
			disableSnapshot: true,
		},
	},
};
