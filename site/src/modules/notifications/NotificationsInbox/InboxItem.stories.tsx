import type { Meta, StoryObj } from "@storybook/react";
import { expect, fn, userEvent, within } from "@storybook/test";
import { MockNotification } from "testHelpers/entities";
import { daysAgo } from "utils/time";
import { InboxItem } from "./InboxItem";

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
		notification: {
			...MockNotification,
			read_at: daysAgo(1),
		},
	},
};

export const Unread: Story = {
	args: {
		notification: {
			...MockNotification,
			read_at: null,
		},
	},
};

export const LongText: Story = {
	args: {
		notification: {
			...MockNotification,
			read_at: null,
			content:
				"Hi User,\n\nTemplate Write Coder on Coder has failed to build 21/330 times over the last week.\n\nReport:\n\n05ebece failed 1 time:\n\nmatifali / dogfood / #379 (https://dev.coder.com/@matifali/dogfood/builds/379)\n\n10f1e0b failed 3 times:\n\ncian / nonix / #585 (https://dev.coder.com/@cian/nonix/builds/585)\ncian / nonix / #582 (https://dev.coder.com/@cian/nonix/builds/582)\nedward / docs / #20 (https://dev.coder.com/@edward/docs/builds/20)\n\n5285c12 failed 1 time:\n\nedward / docs / #26 (https://dev.coder.com/@edward/docs/builds/26)\n\n54745b1 failed 1 time:\n\nedward / docs / #22 (https://dev.coder.com/@edward/docs/builds/22)\n\ne817713 failed 1 time:\n\nedward / docs / #24 (https://dev.coder.com/@edward/docs/builds/24)\n\neb72866 failed 7 times:\n\nammar / blah / #242 (https://dev.coder.com/@ammar/blah/builds/242)\nammar / blah / #241 (https://dev.coder.com/@ammar/blah/builds/241)\nammar / blah / #240 (https://dev.coder.com/@ammar/blah/builds/240)\nammar / blah / #239 (https://dev.coder.com/@ammar/blah/builds/239)\nammar / blah / #238 (https://dev.coder.com/@ammar/blah/builds/238)\nammar / blah / #237 (https://dev.coder.com/@ammar/blah/builds/237)\nammar / blah / #236 (https://dev.coder.com/@ammar/blah/builds/236)\n\nvigorous_hypatia1 failed 7 times:\n\ndean / pog-us / #210 (https://dev.coder.com/@dean/pog-us/builds/210)\ndean / pog-us / #209 (https://dev.coder.com/@dean/pog-us/builds/209)\ndean / pog-us / #208 (https://dev.coder.com/@dean/pog-us/builds/208)\ndean / pog-us / #207 (https://dev.coder.com/@dean/pog-us/builds/207)\ndean / pog-us / #206 (https://dev.coder.com/@dean/pog-us/builds/206)\ndean / pog-us / #205 (https://dev.coder.com/@dean/pog-us/builds/205)\ndean / pog-us / #204 (https://dev.coder.com/@dean/pog-us/builds/204)\n\nWe recommend reviewing these issues to ensure future builds are successful.",
		},
	},
};

export const Markdown: Story = {
	args: {
		notification: {
			...MockNotification,
			read_at: null,
			content:
				"Template **Write Coder on Coder with AI** has failed to build 1/33 times over the last week.\n\n**Report:**\n\n**sweet_cannon7** failed 1 time:\n\n* [edward / coder-on-coder-claude / #34](https://dev.coder.com/@edward/coder-on-coder-claude/builds/34)\n\nWe recommend reviewing these issues to ensure future builds are successful.",
			actions: [
				{
					label: "View workspaces",
					url: "https://dev.coder.com/workspaces?filter=template%3Acoder-with-ai",
				},
			],
			icon: "DEFAULT_TEMPLATE_ICON",
		},
	},
};

export const UnreadFocus: Story = {
	args: {
		notification: {
			...MockNotification,
			read_at: null,
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
			read_at: null,
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
