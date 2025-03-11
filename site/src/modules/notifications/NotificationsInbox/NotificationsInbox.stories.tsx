import type { Meta, StoryObj } from "@storybook/react";
import { expect, fn, userEvent, waitFor, within } from "@storybook/test";
import { MockNotifications, mockApiError } from "testHelpers/entities";
import { withGlobalSnackbar } from "testHelpers/storybook";
import { NotificationsInbox } from "./NotificationsInbox";

const meta: Meta<typeof NotificationsInbox> = {
	title: "modules/notifications/NotificationsInbox/NotificationsInbox",
	component: NotificationsInbox,
	render: (args) => {
		return (
			<div className="w-full max-w-screen-xl p-6 h-[720px]">
				<header className="flex justify-end">
					<NotificationsInbox {...args} />
				</header>
			</div>
		);
	},
};

export default meta;
type Story = StoryObj<typeof NotificationsInbox>;

export const Default: Story = {
	args: {
		defaultOpen: true,
		fetchNotifications: fn(async () => ({
			notifications: MockNotifications,
			unread_count: 2,
		})),
	},
};

export const Failure: Story = {
	args: {
		defaultOpen: true,
		fetchNotifications: fn(() => {
			throw mockApiError({
				message: "Failed to load notifications",
			});
		}),
	},
};

export const FailAndRetry: Story = {
	args: {
		defaultOpen: true,
		fetchNotifications: (() => {
			let count = 0;

			return fn(async () => {
				count += 1;

				if (count === 1) {
					throw mockApiError({
						message: "Failed to load notifications",
					});
				}

				return {
					notifications: MockNotifications,
					unread_count: 2,
				};
			});
		})(),
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await expect(
			body.getByText("Error loading notifications"),
		).toBeInTheDocument();

		const retryButton = body.getByRole("button", { name: /retry/i });
		await userEvent.click(retryButton);
		await waitFor(() => {
			expect(
				body.queryByText("Error loading notifications"),
			).not.toBeInTheDocument();
		});
	},
};

export const MarkAllAsRead: Story = {
	args: {
		defaultOpen: true,
		fetchNotifications: fn(async () => ({
			notifications: MockNotifications,
			unread_count: 2,
		})),
		markAllAsRead: fn(),
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		let unreads = await body.findAllByText(/unread/i);
		await expect(unreads).toHaveLength(2);
		const markAllAsReadButton = body.getByRole("button", {
			name: /mark all as read/i,
		});

		await userEvent.click(markAllAsReadButton);
		unreads = body.queryAllByText(/unread/i);
		await expect(unreads).toHaveLength(0);
	},
};

export const MarkAllAsReadFailure: Story = {
	decorators: [withGlobalSnackbar],
	args: {
		defaultOpen: true,
		fetchNotifications: fn(async () => ({
			notifications: MockNotifications,
			unread_count: 2,
		})),
		markAllAsRead: fn(async () => {
			throw mockApiError({
				message: "Failed to mark all notifications as read",
			});
		}),
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		const markAllAsReadButton = body.getByRole("button", {
			name: /mark all as read/i,
		});
		await userEvent.click(markAllAsReadButton);
		await body.findByText("Failed to mark all notifications as read");
	},
};

export const MarkNotificationAsRead: Story = {
	args: {
		defaultOpen: true,
		fetchNotifications: fn(async () => ({
			notifications: MockNotifications,
			unread_count: 2,
		})),
		markNotificationAsRead: fn(),
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		const notifications = await body.findAllByRole("menuitem");
		const secondNotification = notifications[1];
		within(secondNotification).getByText(/unread/i);

		await userEvent.click(secondNotification);
		const markButton = body.getByRole("button", { name: /mark as read/i });
		await userEvent.click(markButton);
		await expect(within(secondNotification).queryByText(/unread/i)).toBeNull();
	},
};

export const MarkNotificationAsReadFailure: Story = {
	decorators: [withGlobalSnackbar],
	args: {
		defaultOpen: true,
		fetchNotifications: fn(async () => ({
			notifications: MockNotifications,
			unread_count: 2,
		})),
		markNotificationAsRead: fn(() => {
			throw mockApiError({ message: "Failed to mark notification as read" });
		}),
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		const notifications = await body.findAllByRole("menuitem");
		const secondNotification = notifications[1];
		await userEvent.click(secondNotification);
		const markButton = body.getByRole("button", { name: /mark as read/i });
		await userEvent.click(markButton);
		await body.findByText("Failed to mark notification as read");
	},
};
