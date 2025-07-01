import type { Meta, StoryObj } from "@storybook/react";
import { UnreadBadge } from "./UnreadBadge";

const meta: Meta<typeof UnreadBadge> = {
	title: "modules/notifications/NotificationsInbox/UnreadBadge",
	component: UnreadBadge,
};

export default meta;
type Story = StoryObj<typeof UnreadBadge>;

export const Until10: Story = {
	args: {
		count: 3,
	},
};

export const MoreThan10: Story = {
	args: {
		count: 12,
	},
};

export const MoreThan99: Story = {
	args: {
		count: 1000,
	},
};
