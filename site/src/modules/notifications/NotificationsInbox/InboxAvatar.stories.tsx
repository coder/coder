import type { Meta, StoryObj } from "@storybook/react";
import { InboxAvatar } from "./InboxAvatar";

const meta: Meta<typeof InboxAvatar> = {
	title: "modules/notifications/NotificationsInbox/InboxAvatar",
	component: InboxAvatar,
};

export default meta;
type Story = StoryObj<typeof InboxAvatar>;

export const Custom: Story = {
	args: {
		icon: "/icon/git.svg",
	},
};

export const EmptyIcon: Story = {
	args: {
		icon: "",
	},
};

export const FallbackWorkspace: Story = {
	args: {
		icon: "DEFAULT_ICON_WORKSPACE",
	},
};

export const FallbackAccount: Story = {
	args: {
		icon: "DEFAULT_ICON_ACCOUNT",
	},
};

export const FallbackTemplate: Story = {
	args: {
		icon: "DEFAULT_ICON_TEMPLATE",
	},
};

export const FallbackOther: Story = {
	args: {
		icon: "DEFAULT_ICON_OTHER",
	},
};
