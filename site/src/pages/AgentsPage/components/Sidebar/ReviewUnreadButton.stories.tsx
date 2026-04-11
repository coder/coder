import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import type { Chat } from "#/api/typesGenerated";
import { ReviewUnreadButton } from "./ReviewUnreadButton";

const oneHourAgo = new Date(Date.now() - 60 * 60 * 1000).toISOString();

const buildChat = (overrides: Partial<Chat> = {}): Chat => ({
	id: `chat-${Math.random().toString(36).slice(2)}`,
	owner_id: "owner-1",
	title: "Agent",
	status: "completed",
	last_model_config_id: "config-1",
	mcp_server_ids: [],
	labels: {},
	created_at: oneHourAgo,
	updated_at: oneHourAgo,
	archived: false,
	pin_order: 0,
	has_unread: false,
	last_error: null,
	...overrides,
});

const meta: Meta<typeof ReviewUnreadButton> = {
	title: "pages/AgentsPage/ReviewUnreadButton",
	component: ReviewUnreadButton,
	args: {
		onClick: fn(),
	},
	decorators: [
		(Story) => (
			<div className="w-[280px] p-4 bg-surface-primary">
				<Story />
			</div>
		),
	],
};

export default meta;
type Story = StoryObj<typeof ReviewUnreadButton>;

export const NoUnread: Story = {
	args: {
		chatList: [
			buildChat({ has_unread: false }),
			buildChat({ has_unread: false }),
		],
	},
};

export const WithUnread: Story = {
	args: {
		chatList: [
			buildChat({ has_unread: true }),
			buildChat({ has_unread: true }),
			buildChat({ has_unread: true }),
			buildChat({ has_unread: false }),
			buildChat({ has_unread: false }),
		],
	},
};

export const HighCount: Story = {
	args: {
		chatList: Array.from({ length: 12 }, (_, i) =>
			buildChat({ has_unread: i < 10 }),
		),
	},
};
