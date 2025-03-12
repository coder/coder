import type { Meta, StoryObj } from "@storybook/react";
import { UnreadBadge } from "./UnreadBadge";

const meta: Meta<typeof UnreadBadge> = {
	title: "modules/notifications/NotificationsInbox/UnreadBadge",
	component: UnreadBadge,
};

export default meta;
type Story = StoryObj<typeof UnreadBadge>;

export const Default: Story = {
	args: {
		count: 3,
	},
};

export const MoreThanNine: Story = {
	args: {
		count: 12,
	},
};
