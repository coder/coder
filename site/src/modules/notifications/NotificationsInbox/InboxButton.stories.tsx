import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
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

export const BadgeTransition: Story = {
	render: function BadgeTransitionStory() {
		const [count, setCount] = useState(0);

		return (
			<div className="flex items-center gap-4">
				<InboxButton unreadCount={count} />
				<div className="flex gap-2">
					<button
						type="button"
						className="rounded border border-solid border-border px-3 py-1 text-sm bg-surface-primary text-content-primary"
						onClick={() => setCount((c) => c + 1)}
					>
						Add notification
					</button>
					<button
						type="button"
						className="rounded border border-solid border-border px-3 py-1 text-sm bg-surface-primary text-content-primary"
						onClick={() => setCount(0)}
					>
						Clear all
					</button>
				</div>
				<span className="text-sm text-content-secondary">Count: {count}</span>
			</div>
		);
	},
};
